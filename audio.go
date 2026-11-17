package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/cmplx"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/gen2brain/malgo"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/flac"
	"github.com/gopxl/beep/mp3"
	"github.com/gopxl/beep/wav"
)

const outputSampleRate = 48000
const outputChannels = 2
const outputBufferSeconds = 2

type PlaybackState struct {
	Path        string    `json:"path"`
	IsPlaying   bool      `json:"isPlaying"`
	PositionMs  int64     `json:"positionMs"`
	DurationMs  int64     `json:"durationMs"`
	Volume      int       `json:"volume"`
	OutputMode  string    `json:"outputMode"`
	DeviceID    string    `json:"deviceID"`
	OutputReady bool      `json:"outputReady"`
	EqualizerOn bool      `json:"equalizerOn"`
	Equalizer   []float64 `json:"equalizer"`
	Error       string    `json:"error,omitempty"`
}

type AudioDevice struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Default bool   `json:"default"`
}

// pcmRing buffers decoded audio between the decoder and the device callback.
// Frames are tagged with the track generation, so a decoder that is still
// finishing the previous track cannot write into the next one.
type pcmRing struct {
	mu         sync.Mutex
	data       []float32
	readAt     int
	writeAt    int
	frames     int
	capacity   int
	generation uint64
}

func newPCMRing(seconds int) *pcmRing {
	capacity := outputSampleRate * seconds
	return &pcmRing{data: make([]float32, capacity*outputChannels), capacity: capacity}
}

// reset empties the ring and accepts frames only for the given generation.
func (r *pcmRing) reset(generation uint64) {
	r.mu.Lock()
	r.readAt, r.writeAt, r.frames = 0, 0, 0
	r.generation = generation
	r.mu.Unlock()
}

// push stores up to frames frames and returns how many were stored. Frames from
// another generation are dropped.
func (r *pcmRing) push(generation uint64, samples []float32, frames int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	if generation != r.generation {
		return 0
	}
	if available := r.capacity - r.frames; frames > available {
		frames = available
	}
	firstFrames := frames
	if remaining := r.capacity - r.writeAt; firstFrames > remaining {
		firstFrames = remaining
	}
	firstSamples := firstFrames * outputChannels
	copy(r.data[r.writeAt*outputChannels:(r.writeAt+firstFrames)*outputChannels], samples[:firstSamples])
	secondFrames := frames - firstFrames
	if secondFrames > 0 {
		copy(r.data[:secondFrames*outputChannels], samples[firstSamples:firstSamples+secondFrames*outputChannels])
	}
	r.writeAt = (r.writeAt + frames) % r.capacity
	r.frames += frames
	return frames
}

func (r *pcmRing) popIntoFloat32(output []byte, maxFrames int, volume float64) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	frames := maxFrames
	if frames > r.frames {
		frames = r.frames
	}
	if maxOutputFrames := len(output) / (outputChannels * 4); frames > maxOutputFrames {
		frames = maxOutputFrames
	}
	firstFrames := frames
	if remaining := r.capacity - r.readAt; firstFrames > remaining {
		firstFrames = remaining
	}
	writeFloatSamples(output, 0, r.data[r.readAt*outputChannels:(r.readAt+firstFrames)*outputChannels], volume)
	secondFrames := frames - firstFrames
	if secondFrames > 0 {
		writeFloatSamples(output, firstFrames, r.data[:secondFrames*outputChannels], volume)
	}
	r.readAt = (r.readAt + frames) % r.capacity
	r.frames -= frames
	return frames
}

func writeFloatSamples(output []byte, startFrame int, samples []float32, volume float64) {
	start := startFrame * outputChannels * 4
	for index, sample := range samples {
		value := toFloat32(float64(sample) * volume)
		byteIndex := start + index*4
		binary.LittleEndian.PutUint32(output[byteIndex:byteIndex+4], math.Float32bits(value))
	}
}

func (r *pcmRing) empty() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.frames == 0
}

// outputDevice is a started audio output.
type outputDevice interface{ Uninit() }

type AudioPlayer struct {
	deviceMu sync.Mutex
	mu       sync.Mutex
	ctx      *malgo.AllocatedContext
	device   outputDevice
	// openDevice starts an output in a share mode on a device ("" is the
	// Windows default). NewAudioPlayer uses WASAPI.
	openDevice     func(mode, deviceID string) (outputDevice, error)
	ring           *pcmRing
	mode           string
	deviceID       string
	volume         float64
	eqOn           bool
	eqBands        [3]float64
	path           string
	durationMs     int64
	positionFrames int64
	playing        bool
	decoderDone    bool
	ended          bool
	lastErr        string
	generation     uint64
	cancel         context.CancelFunc
	ready          bool
}

func NewAudioPlayer() *AudioPlayer {
	p := &AudioPlayer{ring: newPCMRing(outputBufferSeconds), mode: "shared", volume: 0.72}
	p.openDevice = p.openWASAPIDevice
	ctx, err := malgo.InitContext([]malgo.Backend{malgo.BackendWasapi}, malgo.ContextConfig{}, nil)
	if err != nil {
		p.lastErr = "No se pudo iniciar el audio WASAPI: " + err.Error()
		return p
	}
	p.ctx = ctx
	_, _, _ = p.SetOutput("shared", "")
	return p
}

func (p *AudioPlayer) Close() {
	p.stopTrack()
	p.deviceMu.Lock()
	p.closeDevice()
	if p.ctx != nil {
		_ = p.ctx.Uninit()
		p.ctx.Free()
		p.ctx = nil
	}
	p.deviceMu.Unlock()
}

// closeDevice stops the current output. The caller holds deviceMu.
func (p *AudioPlayer) closeDevice() {
	if p.device != nil {
		p.device.Uninit()
		p.device = nil
	}
}

// SetOutput opens deviceID ("" for the Windows default) in the given mode and
// returns the mode and device actually in use. Exclusive mode falls back to
// shared mode on the same device. If the device cannot be opened at all, the
// previous output is restored and the error is returned.
func (p *AudioPlayer) SetOutput(mode, deviceID string) (string, string, error) {
	if mode != "exclusive" {
		mode = "shared"
	}
	p.deviceMu.Lock()
	defer p.deviceMu.Unlock()
	p.mu.Lock()
	previousMode, previousID, wasReady := p.mode, p.deviceID, p.ready
	p.mu.Unlock()

	p.closeDevice()
	actualMode, err := p.openOutput(mode, deviceID)
	actualID := deviceID
	restored := false
	if err != nil && wasReady && deviceID != previousID {
		if restoredMode, restoreErr := p.openOutput(previousMode, previousID); restoreErr == nil {
			actualMode, actualID, restored = restoredMode, previousID, true
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.mode, p.deviceID, p.ready = actualMode, actualID, p.device != nil
	switch {
	case restored:
		p.lastErr = "No se pudo cambiar el dispositivo de audio: " + err.Error()
	case err != nil:
		p.lastErr = "No se pudo iniciar el dispositivo de audio: " + err.Error()
	case mode == "exclusive" && actualMode == "shared":
		p.lastErr = "El dispositivo no admite modo exclusivo; se mantuvo la salida compartida."
	default:
		p.lastErr = ""
	}
	return p.mode, p.deviceID, err
}

// openOutput starts deviceID in mode, falling back from exclusive to shared.
// It returns the mode in use, or the requested mode and the error.
func (p *AudioPlayer) openOutput(mode, deviceID string) (string, error) {
	device, err := p.openDevice(mode, deviceID)
	if err != nil && mode == "exclusive" {
		if shared, sharedErr := p.openDevice("shared", deviceID); sharedErr == nil {
			p.device = shared
			return "shared", nil
		}
	}
	if err != nil {
		return mode, err
	}
	p.device = device
	return mode, nil
}

func (p *AudioPlayer) Devices() []AudioDevice {
	p.deviceMu.Lock()
	defer p.deviceMu.Unlock()
	if p.ctx == nil {
		return []AudioDevice{}
	}
	devices, err := p.ctx.Devices(malgo.Playback)
	if err != nil {
		return []AudioDevice{}
	}
	result := make([]AudioDevice, 0, len(devices))
	for _, device := range devices {
		result = append(result, AudioDevice{ID: device.ID.String(), Name: device.Name(), Default: device.IsDefault == 1})
	}
	return result
}

// openWASAPIDevice starts a WASAPI output. The caller holds deviceMu.
func (p *AudioPlayer) openWASAPIDevice(mode, deviceID string) (outputDevice, error) {
	if p.ctx == nil {
		ctx, err := malgo.InitContext([]malgo.Backend{malgo.BackendWasapi}, malgo.ContextConfig{}, nil)
		if err != nil {
			return nil, err
		}
		p.ctx = ctx
	}
	config := malgo.DefaultDeviceConfig(malgo.Playback)
	config.Playback.Format = malgo.FormatF32
	config.Playback.Channels = outputChannels
	config.SampleRate = outputSampleRate
	if deviceID != "" {
		id, err := malgo.ParseDeviceID(deviceID)
		if err != nil {
			return nil, fmt.Errorf("identificador de dispositivo inválido: %w", err)
		}
		config.Playback.DeviceID = unsafe.Pointer(&id)
	}
	if mode == "exclusive" {
		config.Playback.ShareMode = malgo.Exclusive
	} else {
		config.Playback.ShareMode = malgo.Shared
	}
	device, err := malgo.InitDevice(p.ctx.Context, config, malgo.DeviceCallbacks{Data: p.render})
	if err != nil {
		return nil, err
	}
	if err := device.Start(); err != nil {
		device.Uninit()
		return nil, err
	}
	return device, nil
}

func (p *AudioPlayer) Play(path string, positionMs int64) error {
	stream, format, err := openAudio(path)
	if err != nil {
		p.mu.Lock()
		p.lastErr = err.Error()
		p.mu.Unlock()
		return err
	}
	if format.SampleRate <= 0 {
		stream.Close()
		return fmt.Errorf("frecuencia de muestreo inválida")
	}
	p.mu.Lock()
	ready, outputError := p.ready, p.lastErr
	p.mu.Unlock()
	if !ready {
		stream.Close()
		if outputError == "" {
			outputError = "No hay un dispositivo de audio disponible. Revisa Ajustes > Audio."
		}
		return fmt.Errorf("%s", outputError)
	}
	totalSamples := stream.Len()
	seekSamples := int(float64(format.SampleRate) * float64(positionMs) / 1000)
	if seekSamples < 0 {
		seekSamples = 0
	}
	if seekSamples > totalSamples {
		seekSamples = totalSamples
	}
	if err := stream.Seek(seekSamples); err != nil {
		stream.Close()
		return fmt.Errorf("no se pudo buscar la posición solicitada: %w", err)
	}
	durationMs := audioDurationMs(totalSamples, format.SampleRate)

	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
	}
	p.generation++
	generation := p.generation
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.path = path
	p.durationMs = durationMs
	p.positionFrames = positionMs * outputSampleRate / 1000
	p.playing = true
	p.decoderDone = false
	p.ended = false
	p.lastErr = ""
	// Emptying the ring while holding mu keeps the device callback from playing
	// leftovers of the previous track as part of this one.
	p.ring.reset(generation)
	p.mu.Unlock()
	go p.decode(ctx, generation, stream, format)
	return nil
}

func openAudio(path string) (beep.StreamSeekCloser, beep.Format, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("no se puede abrir el archivo: %w", err)
	}
	var stream beep.StreamSeekCloser
	var format beep.Format
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		stream, format, err = mp3.Decode(file)
	case ".wav":
		stream, format, err = wav.Decode(file)
	case ".flac":
		stream, format, err = flac.Decode(file)
	default:
		err = fmt.Errorf("formato no compatible. Usa MP3, WAV o FLAC")
	}
	if err != nil {
		_ = file.Close()
		return nil, beep.Format{}, fmt.Errorf("no se pudo decodificar la canción: %w", err)
	}
	return stream, format, nil
}

func audioDurationMs(samples int, sampleRate beep.SampleRate) int64 {
	if samples <= 0 || sampleRate <= 0 {
		return 0
	}
	return int64(samples) * 1000 / int64(sampleRate)
}

func (p *AudioPlayer) decode(ctx context.Context, generation uint64, stream beep.StreamSeekCloser, format beep.Format) {
	defer stream.Close()
	resampler := beep.Resample(3, format.SampleRate, beep.SampleRate(outputSampleRate), stream)
	processor := newEQProcessor(outputSampleRate)
	floatSamples := make([][2]float64, 1024)
	pcm := make([]float32, len(floatSamples)*outputChannels)
	for {
		if ctx.Err() != nil || !p.isCurrent(generation) {
			return
		}
		if !p.isPlaying(generation) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(12 * time.Millisecond):
				continue
			}
		}
		n, ok := resampler.Stream(floatSamples)
		if n > 0 {
			enabled, bands := p.equalizerSettings()
			processor.Set(enabled, bands)
			for i := 0; i < n; i++ {
				left, right := processor.Process(floatSamples[i][0], floatSamples[i][1])
				pcm[i*2] = float32(left)
				pcm[i*2+1] = float32(right)
			}
			remaining := n
			written := 0
			for remaining > 0 {
				if ctx.Err() != nil || !p.isCurrent(generation) {
					return
				}
				if !p.isPlaying(generation) {
					select {
					case <-ctx.Done():
						return
					case <-time.After(12 * time.Millisecond):
						continue
					}
				}
				count := p.ring.push(generation, pcm[written*2:written*2+remaining*2], remaining)
				written += count
				remaining -= count
				if count == 0 {
					select {
					case <-ctx.Done():
						return
					case <-time.After(3 * time.Millisecond):
					}
				}
			}
		}
		if !ok {
			if err := resampler.Err(); err != nil {
				p.mu.Lock()
				if p.generation == generation {
					p.lastErr = "Error de lectura de audio: " + err.Error()
				}
				p.mu.Unlock()
			}
			p.mu.Lock()
			if p.generation == generation {
				p.decoderDone = true
			}
			p.mu.Unlock()
			return
		}
	}
}

func (p *AudioPlayer) render(output, _ []byte, frameCount uint32) {
	for i := range output {
		output[i] = 0
	}
	frames := int(frameCount)
	if maxFrames := len(output) / (outputChannels * 4); frames > maxFrames {
		frames = maxFrames
	}
	// Reading the ring and advancing the position under one lock means a track
	// change cannot happen in between and inherit this block's frames.
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.playing {
		return
	}
	p.positionFrames += int64(p.ring.popIntoFloat32(output, frames, p.volume))
	if p.decoderDone && p.ring.empty() {
		p.playing = false
		p.ended = true
	}
}

func (p *AudioPlayer) Toggle() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.path == "" {
		return false
	}
	p.playing = !p.playing
	return p.playing
}

func (p *AudioPlayer) Pause() {
	p.mu.Lock()
	p.playing = false
	p.mu.Unlock()
}

func (p *AudioPlayer) SetVolume(value int) {
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	p.mu.Lock()
	p.volume = float64(value) / 100
	p.mu.Unlock()
}

func (p *AudioPlayer) SetEqualizer(enabled bool, bands []float64) {
	var normalized [3]float64
	for i := 0; i < len(normalized) && i < len(bands); i++ {
		value := bands[i]
		if value < -12 {
			value = -12
		}
		if value > 12 {
			value = 12
		}
		normalized[i] = value
	}
	p.mu.Lock()
	p.eqOn, p.eqBands = enabled, normalized
	p.mu.Unlock()
}

// SeekTo restarts the current track at positionMs, keeping it paused if it was.
func (p *AudioPlayer) SeekTo(positionMs int64) {
	p.mu.Lock()
	path := p.path
	wasPlaying := p.playing
	if positionMs < 0 {
		positionMs = 0
	}
	if positionMs > p.durationMs {
		positionMs = p.durationMs
	}
	p.mu.Unlock()
	if path != "" {
		if err := p.Play(path, positionMs); err == nil && !wasPlaying {
			p.Pause()
		}
	}
}

func (p *AudioPlayer) State() PlaybackState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return PlaybackState{
		Path: p.path, IsPlaying: p.playing,
		PositionMs: p.positionFrames * 1000 / outputSampleRate,
		DurationMs: p.durationMs,
		Volume:     int(math.Round(p.volume * 100)),
		OutputMode: p.mode, DeviceID: p.deviceID, OutputReady: p.ready,
		EqualizerOn: p.eqOn, Equalizer: []float64{p.eqBands[0], p.eqBands[1], p.eqBands[2]},
		Error: p.lastErr,
	}
}

func (p *AudioPlayer) TakeEnded() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	ended := p.ended
	p.ended = false
	return ended
}

func (p *AudioPlayer) stopTrack() {
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.generation++
	p.path = ""
	p.playing = false
	p.decoderDone = false
	p.ended = false
	p.positionFrames = 0
	p.durationMs = 0
	p.ring.reset(p.generation)
	p.mu.Unlock()
}

func (p *AudioPlayer) isCurrent(generation uint64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.generation == generation
}

func (p *AudioPlayer) isPlaying(generation uint64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.generation == generation && p.playing
}

func (p *AudioPlayer) equalizerSettings() (bool, [3]float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.eqOn, p.eqBands
}

func toFloat32(sample float64) float32 {
	if sample > 1 {
		sample = 1
	} else if sample < -1 {
		sample = -1
	}
	return float32(sample)
}

type biquad struct {
	b0, b1, b2 float64
	a1, a2     float64
	z1, z2     float64
}

func makePeakingEQ(sampleRate, frequency, gainDB, q float64) biquad {
	if gainDB == 0 {
		return biquad{b0: 1}
	}
	if frequency >= sampleRate/2 {
		frequency = sampleRate / 2 * 0.95
	}
	a := math.Pow(10, gainDB/40)
	w0 := 2 * math.Pi * frequency / sampleRate
	alpha := math.Sin(w0) / (2 * q)
	cosine := math.Cos(w0)
	a0 := 1 + alpha/a
	return biquad{
		b0: (1 + alpha*a) / a0,
		b1: (-2 * cosine) / a0,
		b2: (1 - alpha*a) / a0,
		a1: (-2 * cosine) / a0,
		a2: (1 - alpha/a) / a0,
	}
}

func (b *biquad) process(input float64) float64 {
	output := b.b0*input + b.z1
	b.z1 = b.b1*input - b.a1*output + b.z2
	b.z2 = b.b2*input - b.a2*output
	return output
}

// magnitude returns the filter's gain at angular frequency w (radians/sample).
func (b biquad) magnitude(w float64) float64 {
	z1 := cmplx.Exp(complex(0, -w))
	z2 := z1 * z1
	numerator := complex(b.b0, 0) + complex(b.b1, 0)*z1 + complex(b.b2, 0)*z2
	denominator := 1 + complex(b.a1, 0)*z1 + complex(b.a2, 0)*z2
	return cmplx.Abs(numerator / denominator)
}

// eqHeadroomMarginDB covers the peak falling between the frequencies checked.
const eqHeadroomMarginDB = 0.1

type eqProcessor struct {
	sampleRate float64
	enabled    bool
	bands      [3]float64
	left       [3]biquad
	right      [3]biquad
	// preamp lowers the input by the equalizer's largest boost, so boosted
	// frequencies keep their shape instead of clipping at full scale.
	preamp float64
}

func newEQProcessor(sampleRate int) *eqProcessor {
	return &eqProcessor{sampleRate: float64(sampleRate), preamp: 1}
}

func (e *eqProcessor) Set(enabled bool, bands [3]float64) {
	if e.enabled == enabled && e.bands == bands {
		return
	}
	e.enabled, e.bands = enabled, bands
	frequencies := [3]float64{100, 1000, 10000}
	for i, frequency := range frequencies {
		e.left[i] = makePeakingEQ(e.sampleRate, frequency, bands[i], 0.707)
		e.right[i] = makePeakingEQ(e.sampleRate, frequency, bands[i], 0.707)
	}
	e.preamp = 1
	if peak := cascadePeakDB(e.left[:], e.sampleRate); peak > 0 {
		e.preamp = math.Pow(10, -(peak+eqHeadroomMarginDB)/20)
	}
}

// cascadePeakDB returns the largest gain of the filters in series between
// 20 Hz and 20 kHz, in decibels.
func cascadePeakDB(filters []biquad, sampleRate float64) float64 {
	const points = 512
	peak := math.Inf(-1)
	for i := 0; i <= points; i++ {
		frequency := 20 * math.Pow(1000, float64(i)/points)
		if frequency >= sampleRate/2 {
			break
		}
		w := 2 * math.Pi * frequency / sampleRate
		gain := 1.0
		for _, filter := range filters {
			gain *= filter.magnitude(w)
		}
		peak = math.Max(peak, 20*math.Log10(gain))
	}
	return peak
}

func (e *eqProcessor) Process(left, right float64) (float64, float64) {
	if !e.enabled {
		return left, right
	}
	left, right = left*e.preamp, right*e.preamp
	for i := range e.left {
		left = e.left[i].process(left)
		right = e.right[i].process(right)
	}
	return left, right
}

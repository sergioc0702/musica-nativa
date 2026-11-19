package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
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

// popInto moves up to maxFrames frames into dst and returns how many it moved.
func (r *pcmRing) popInto(dst []float32, maxFrames int) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	frames := min(maxFrames, r.frames, len(dst)/outputChannels)
	firstFrames := min(frames, r.capacity-r.readAt)
	copy(dst, r.data[r.readAt*outputChannels:(r.readAt+firstFrames)*outputChannels])
	if secondFrames := frames - firstFrames; secondFrames > 0 {
		copy(dst[firstFrames*outputChannels:], r.data[:secondFrames*outputChannels])
	}
	r.readAt = (r.readAt + frames) % r.capacity
	r.frames -= frames
	return frames
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
	openDevice func(mode, deviceID string) (outputDevice, error)
	ring       *pcmRing
	mode       string
	deviceID   string
	volume     float64
	eqOn       bool
	eqBands    [eqBandCount]float64
	// eq shapes the sound in render, as it leaves for the device; scratch
	// holds the samples of one device block while that happens.
	eq             eqProcessor
	scratch        []float32
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
			for i := 0; i < n; i++ {
				pcm[i*2] = float32(floatSamples[i][0])
				pcm[i*2+1] = float32(floatSamples[i][1])
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
	if needed := frames * outputChannels; len(p.scratch) < needed {
		p.scratch = make([]float32, needed) // only when the device asks for a larger block
	}
	read := p.ring.popInto(p.scratch, frames)
	samples := p.scratch[:read*outputChannels]
	// Equalizing here, after the queue, makes a change audible right away.
	p.eq.process(samples)
	for index, sample := range samples {
		binary.LittleEndian.PutUint32(output[index*4:index*4+4], math.Float32bits(toFloat32(float64(sample)*p.volume)))
	}
	p.positionFrames += int64(read)
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

// SetEqualizer changes the equalizer's ten bands (dB). The change is heard
// within milliseconds, because render applies it as the audio leaves.
func (p *AudioPlayer) SetEqualizer(enabled bool, values []float64) {
	bands := eqBands(values)
	preamp := eqPreampDB(bands) // outside the lock: render waits on it
	p.mu.Lock()
	p.eqOn, p.eqBands = enabled, bands
	p.eq.setTarget(enabled, bands, preamp)
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
		EqualizerOn: p.eqOn, Equalizer: append([]float64(nil), p.eqBands[:]...),
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

func toFloat32(sample float64) float32 {
	if sample > 1 {
		sample = 1
	} else if sample < -1 {
		sample = -1
	}
	return float32(sample)
}

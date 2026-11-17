package main

import (
	"encoding/binary"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestOpenAudioStreamsWaveAndReportsSampleRate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quiet.wav")
	if err := writeTestWAV(path, 8000, 8000); err != nil {
		t.Fatal(err)
	}
	stream, format, err := openAudio(path)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if format.SampleRate != 8000 || format.NumChannels != 1 {
		t.Fatalf("format = %+v, want 8000 Hz mono", format)
	}
	if stream.Len() != 8000 {
		t.Fatalf("stream length = %d, want 8000 samples", stream.Len())
	}
	samples := make([][2]float64, 16)
	n, ok := stream.Stream(samples)
	if n != len(samples) || !ok {
		t.Fatalf("decoded %d samples (ok=%v), want 16", n, ok)
	}
}

func TestOpenAudioDecodesMP3AndFLAC(t *testing.T) {
	for _, name := range []string{"sample.mp3", "sample.flac"} {
		t.Run(name, func(t *testing.T) {
			stream, format, err := openAudio(filepath.Join("audio-testdata", name))
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			if stream.Len() <= 0 || format.SampleRate <= 0 {
				t.Fatalf("invalid decoded audio: %d samples at %d Hz", stream.Len(), format.SampleRate)
			}
			samples := make([][2]float64, 32)
			n, ok := stream.Stream(samples)
			if n == 0 || !ok {
				t.Fatalf("decoder returned %d samples (ok=%v)", n, ok)
			}
		})
	}
}

func TestAudioDurationDoesNotShrinkAfterSeek(t *testing.T) {
	path := filepath.Join(t.TempDir(), "duration.wav")
	if err := writeTestWAV(path, 8000, 8000); err != nil {
		t.Fatal(err)
	}
	stream, format, err := openAudio(path)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if err := stream.Seek(4000); err != nil {
		t.Fatal(err)
	}
	if got := audioDurationMs(stream.Len(), format.SampleRate); got != 1000 {
		t.Fatalf("duration after seeking = %d ms, want original 1000 ms", got)
	}
}

func TestPCMRingReadsWrappedFramesInOneBlock(t *testing.T) {
	ring := &pcmRing{data: make([]float32, 6), capacity: 3}
	if got := ring.push(0, []float32{0.2, -0.2, 0.4, -0.4}, 2); got != 2 {
		t.Fatalf("first push = %d frames, want 2", got)
	}
	first := make([]byte, outputChannels*4)
	if got := ring.popIntoFloat32(first, 1, 1); got != 1 {
		t.Fatalf("first pop = %d frames, want 1", got)
	}
	if got := ring.push(0, []float32{0.6, -0.6, 0.8, -0.8}, 2); got != 2 {
		t.Fatalf("wrapped push = %d frames, want 2", got)
	}
	output := make([]byte, 3*outputChannels*4)
	if got := ring.popIntoFloat32(output, 3, .5); got != 3 {
		t.Fatalf("wrapped pop = %d frames, want 3", got)
	}
	want := []float32{0.2, -0.2, 0.3, -0.3, 0.4, -0.4}
	if got := math.Float32frombits(binary.LittleEndian.Uint32(first[:4])); got != 0.2 {
		t.Fatalf("first sample = %v, want 0.2", got)
	}
	for index, value := range want {
		got := math.Float32frombits(binary.LittleEndian.Uint32(output[index*4 : index*4+4]))
		if got != value {
			t.Fatalf("sample %d = %v, want %v", index, got, value)
		}
	}
}

func TestOpenAudioRejectsUnsupportedFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "song.m4a")
	if err := os.WriteFile(path, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openAudio(path); err == nil {
		t.Fatal("expected unsupported-format error")
	}
}

func TestOutputSampleClampsAndEqualizerBypassesWhenDisabled(t *testing.T) {
	if got := toFloat32(2); got != 1 {
		t.Fatalf("positive clipping = %v, want 1", got)
	}
	if got := toFloat32(-2); got != -1 {
		t.Fatalf("negative clipping = %v, want -1", got)
	}
	processor := newEQProcessor(outputSampleRate)
	processor.Set(true, [3]float64{6, -2, 3})
	left, right := processor.Process(.1, -.1)
	if math.IsNaN(left) || math.IsInf(left, 0) || math.IsNaN(right) || math.IsInf(right, 0) {
		t.Fatalf("EQ returned invalid samples %v, %v", left, right)
	}
	processor = newEQProcessor(outputSampleRate)
	left, right = processor.Process(.2, -.3)
	if left != .2 || right != -.3 {
		t.Fatalf("disabled EQ changed the signal to %v, %v", left, right)
	}
}

func TestRingDropsFramesQueuedForPreviousTrack(t *testing.T) {
	ring := newPCMRing(1)
	ring.reset(1)
	ring.push(1, []float32{0.1, 0.1}, 1)
	ring.reset(2)
	if written := ring.push(1, []float32{0.9, 0.9}, 1); written != 0 {
		t.Fatalf("the previous track's decoder wrote %d frames into the new track", written)
	}
	ring.push(2, []float32{0.2, -0.2}, 1)
	output := make([]byte, 2*outputChannels*4)
	if frames := ring.popIntoFloat32(output, 2, 1); frames != 1 {
		t.Fatalf("played %d frames; want only the new track's frame", frames)
	}
	if got := math.Float32frombits(binary.LittleEndian.Uint32(output[:4])); got != 0.2 {
		t.Fatalf("first sample = %v, want 0.2 from the new track", got)
	}
}

func TestEqualizerBoostLeavesHeadroom(t *testing.T) {
	processor := newEQProcessor(outputSampleRate)
	processor.Set(true, [3]float64{12, 12, 12})
	for _, frequency := range []float64{100, 316, 1000, 3162, 10000} {
		if peak := peakAfterEQ(processor, frequency, 1); peak > 1 {
			t.Errorf("a full-scale %v Hz tone peaks at %.3f with every band at +12 dB; want at most 1", frequency, peak)
		}
	}
}

func TestEqualizerCutKeepsLevelAwayFromItsBand(t *testing.T) {
	processor := newEQProcessor(outputSampleRate)
	processor.Set(true, [3]float64{-6, 0, 0})
	if peak := peakAfterEQ(processor, 10000, 0.5); math.Abs(peak-0.5) > 0.01 {
		t.Fatalf("a 10 kHz tone at 0.5 came out at %.3f with only the bass cut", peak)
	}
}

// peakAfterEQ plays one second of a sine tone and returns the largest output
// sample of the last half second, after the filters have settled.
func peakAfterEQ(processor *eqProcessor, frequency, amplitude float64) float64 {
	peak := 0.0
	for i := 0; i < outputSampleRate; i++ {
		sample := amplitude * math.Sin(2*math.Pi*frequency*float64(i)/outputSampleRate)
		left, _ := processor.Process(sample, sample)
		if i >= outputSampleRate/2 {
			peak = math.Max(peak, math.Abs(left))
		}
	}
	return peak
}

type fakeDevice struct{}

func (fakeDevice) Uninit() {}

// fakeOutputs stands in for WASAPI: it records each mode@device opened and
// fails the combinations listed in failing.
type fakeOutputs struct {
	mu      sync.Mutex
	opened  []string
	failing map[string]bool
}

func (f *fakeOutputs) open(mode, deviceID string) (outputDevice, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := mode + "@" + deviceID
	f.opened = append(f.opened, key)
	if f.failing[key] {
		return nil, errors.New("dispositivo no disponible")
	}
	return fakeDevice{}, nil
}

func newTestPlayer(outputs *fakeOutputs) *AudioPlayer {
	return &AudioPlayer{ring: newPCMRing(outputBufferSeconds), mode: "shared", volume: 0.72, openDevice: outputs.open}
}

func TestSetOutputOpensDeviceInRequestedMode(t *testing.T) {
	player := newTestPlayer(&fakeOutputs{})
	mode, device, err := player.SetOutput("exclusive", "usb-dac")
	state := player.State()
	if err != nil || mode != "exclusive" || device != "usb-dac" || !state.OutputReady || state.Error != "" {
		t.Fatalf("SetOutput = %s, %q, %v; state %+v", mode, device, err, state)
	}
}

func TestSetOutputFallsBackToSharedOnSameDevice(t *testing.T) {
	outputs := &fakeOutputs{failing: map[string]bool{"exclusive@usb-dac": true}}
	player := newTestPlayer(outputs)
	mode, device, err := player.SetOutput("exclusive", "usb-dac")
	state := player.State()
	if err != nil || mode != "shared" || device != "usb-dac" || !state.OutputReady {
		t.Fatalf("SetOutput = %s, %q, %v; want shared output on usb-dac", mode, device, err)
	}
	if state.Error == "" {
		t.Fatal("the listener is not told that exclusive mode was unavailable")
	}
}

func TestSetOutputKeepsPreviousDeviceWhenNewOneFails(t *testing.T) {
	outputs := &fakeOutputs{failing: map[string]bool{"shared@broken": true}}
	player := newTestPlayer(outputs)
	if _, _, err := player.SetOutput("shared", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := player.SetOutput("shared", "broken"); err == nil {
		t.Fatal("expected an error for a device that cannot be opened")
	}
	state := player.State()
	if state.DeviceID != "" || !state.OutputReady || state.Error == "" {
		t.Fatalf("after a failed switch the state is %+v; want the default device, ready, with a message", state)
	}
}

func writeTestWAV(path string, sampleRate, frames int) error {
	dataBytes := frames * 2
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(36+dataBytes)); err != nil {
		return err
	}
	if _, err := file.Write([]byte("WAVEfmt ")); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(16)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(sampleRate)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(sampleRate*2)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(2)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(16)); err != nil {
		return err
	}
	if _, err := file.Write([]byte("data")); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(dataBytes)); err != nil {
		return err
	}
	return binary.Write(file, binary.LittleEndian, make([]int16, frames))
}

package main

import (
	"encoding/binary"
	"errors"
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
	first := make([]float32, outputChannels)
	if got := ring.popInto(first, 1); got != 1 || first[0] != 0.2 {
		t.Fatalf("first pop = %d frames starting %v, want 1 frame starting 0.2", got, first[0])
	}
	if got := ring.push(0, []float32{0.6, -0.6, 0.8, -0.8}, 2); got != 2 {
		t.Fatalf("wrapped push = %d frames, want 2", got)
	}
	output := make([]float32, 3*outputChannels)
	if got := ring.popInto(output, 3); got != 3 {
		t.Fatalf("wrapped pop = %d frames, want 3", got)
	}
	want := []float32{0.4, -0.4, 0.6, -0.6, 0.8, -0.8}
	for index, value := range want {
		if output[index] != value {
			t.Fatalf("sample %d = %v, want %v", index, output[index], value)
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

func TestOutputSampleClamps(t *testing.T) {
	if got := toFloat32(2); got != 1 {
		t.Fatalf("positive clipping = %v, want 1", got)
	}
	if got := toFloat32(-2); got != -1 {
		t.Fatalf("negative clipping = %v, want -1", got)
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
	output := make([]float32, 2*outputChannels)
	if frames := ring.popInto(output, 2); frames != 1 {
		t.Fatalf("played %d frames; want only the new track's frame", frames)
	}
	if output[0] != 0.2 {
		t.Fatalf("first sample = %v, want 0.2 from the new track", output[0])
	}
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

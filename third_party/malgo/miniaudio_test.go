package malgo_test

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"math"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/gen2brain/malgo"
)

func TestCapturePlayback(t *testing.T) {
	onLog := func(message string) {
		fmt.Fprintf(ioutil.Discard, message)
	}

	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, onLog)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = 2
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = 2
	deviceConfig.SampleRate = 44100
	deviceConfig.Alsa.NoMMap = 1

	var playbackSampleCount uint32
	var capturedSampleCount uint32
	pCapturedSamples := make([]byte, 0)

	sizeInBytes := uint32(malgo.SampleSizeInBytes(deviceConfig.Playback.Format))
	onRecvFrames := func(outpuSamples, inputSamples []byte, framecount uint32) {
		sampleCount := framecount * deviceConfig.Playback.Channels * sizeInBytes

		newCapturedSampleCount := capturedSampleCount + sampleCount

		pCapturedSamples = append(pCapturedSamples, inputSamples...)

		capturedSampleCount = newCapturedSampleCount
	}

	captureCallbacks := malgo.DeviceCallbacks{
		Data: onRecvFrames,
	}
	captureDeviceConfig := deviceConfig
	captureDeviceConfig.DeviceType = malgo.Capture
	device, err := malgo.InitDevice(ctx.Context, captureDeviceConfig, captureCallbacks)
	if err != nil {
		t.Fatal(err)
	}

	if device.Type() != malgo.Capture {
		t.Errorf("wrong device type")
	}

	if device.PlaybackFormat() != malgo.FormatS16 {
		t.Errorf("wrong format")
	}

	if device.PlaybackChannels() != 2 {
		t.Errorf("wrong number of channels")
	}

	if device.SampleRate() != 44100 {
		t.Errorf("wrong samplerate")
	}

	err = device.Start()
	if err != nil {
		t.Fatal(err)
	}

	if !device.IsStarted() {
		t.Fatalf("device not started")
	}

	time.Sleep(1 * time.Second)

	device.Uninit()

	onSendFrames := func(outputSamples, inputSamples []byte, framecount uint32) {
		samplesToRead := framecount * deviceConfig.Playback.Channels * sizeInBytes
		if samplesToRead > capturedSampleCount-playbackSampleCount {
			samplesToRead = capturedSampleCount - playbackSampleCount
		}

		copy(outputSamples, pCapturedSamples[playbackSampleCount:playbackSampleCount+samplesToRead])

		playbackSampleCount += samplesToRead
	}

	playbackCallbacks := malgo.DeviceCallbacks{
		Data: onSendFrames,
	}

	playbackDeviceConfig := deviceConfig
	playbackDeviceConfig.DeviceType = malgo.Playback
	device, err = malgo.InitDevice(ctx.Context, playbackDeviceConfig, playbackCallbacks)
	if err != nil {
		t.Fatal(err)
	}

	if device.Type() != malgo.Playback {
		t.Errorf("wrong device type")
	}

	err = device.Start()
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)

	device.Uninit()
}

func TestErrors(t *testing.T) {
	_, err := malgo.InitContext([]malgo.Backend{malgo.Backend(99)}, malgo.ContextConfig{}, nil)
	if err == nil {
		t.Fatalf("context init with invalid backend")
	}

	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	onSendFrames := func(outputSamples, inputSamples []byte, framecount uint32) {
	}

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = 255
	deviceConfig.SampleRate = 44100

	_, err = malgo.InitDevice(ctx.Context, deviceConfig, malgo.DeviceCallbacks{})
	if err == nil {
		t.Fatalf("device init with invalid config")
	}

	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = 2
	deviceConfig.SampleRate = 44100

	dev, err := malgo.InitDevice(ctx.Context, deviceConfig, malgo.DeviceCallbacks{
		Data: onSendFrames,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = dev.Start()
	if err != nil {
		t.Fatal(err)
	}

	err = dev.Start()
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)

	err = dev.Stop()
	if err != nil {
		t.Fatal(err)
	}

	err = dev.Stop()
	if err != nil {
		t.Fatal(err)
	}

	dev.Uninit()
}

func TestDeviceID(t *testing.T) {
	ctx, err := malgo.InitContext([]malgo.Backend{malgo.BackendNull}, malgo.ContextConfig{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	infos, err := ctx.Devices(malgo.Playback)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) == 0 {
		t.Fatal("no null playback device")
	}

	config := malgo.DefaultDeviceConfig(malgo.Playback)
	config.Playback.Format = malgo.FormatS16
	config.Playback.Channels = 2
	config.SampleRate = 44100
	config.Playback.DeviceID = unsafe.Pointer(&infos[0].ID)

	device, err := malgo.InitDevice(ctx.Context, config, malgo.DeviceCallbacks{})
	if err != nil {
		t.Fatal(err)
	}
	device.Uninit()
}

func TestDataConverter(t *testing.T) {
	config := malgo.DefaultDataConverterConfig(malgo.FormatS16, malgo.FormatF32, 2, 1, 48000, 48000)
	conv, err := malgo.InitDataConverter(config)
	if err != nil {
		t.Fatal(err)
	}
	defer conv.Uninit()

	in := make([]byte, 100*malgo.FrameSizeInBytes(malgo.FormatS16, 2))
	for i := 0; i < len(in); i += 2 {
		binary.LittleEndian.PutUint16(in[i:], 16384)
	}
	out := make([]byte, 100*malgo.FrameSizeInBytes(malgo.FormatF32, 1))

	read, written, err := conv.ProcessFrames(in, out)
	if err != nil {
		t.Fatal(err)
	}
	if read != 100 || written != 100 {
		t.Fatalf("read %d, written %d, want 100, 100", read, written)
	}
	for i := 0; i < len(out); i += 4 {
		if v := math.Float32frombits(binary.LittleEndian.Uint32(out[i:])); v != 0.5 {
			t.Fatalf("frame %d is %v, want 0.5", i/4, v)
		}
	}

	config = malgo.DefaultDataConverterConfig(malgo.FormatS16, malgo.FormatS16, 1, 1, 48000, 44100)
	resampler, err := malgo.InitDataConverter(config)
	if err != nil {
		t.Fatal(err)
	}
	defer resampler.Uninit()

	expected, err := resampler.ExpectedOutputFrameCount(4800)
	if err != nil {
		t.Fatal(err)
	}
	if expected < 4400 || expected > 4420 {
		t.Fatalf("expected output %d frames, want about 4410", expected)
	}

	in = make([]byte, 4800*2)
	out = make([]byte, 8000*2)
	read, written, err = resampler.ProcessFrames(in, out)
	if err != nil {
		t.Fatal(err)
	}
	if read != 4800 || written < 4400 || written > 4420 {
		t.Fatalf("read %d, written %d, want 4800, about 4410", read, written)
	}

	if _, err := malgo.InitDataConverter(malgo.DataConverterConfig{}); err == nil {
		t.Fatal("init with an empty config")
	}
}

func TestParseDeviceID(t *testing.T) {
	ctx, err := malgo.InitContext([]malgo.Backend{malgo.BackendNull}, malgo.ContextConfig{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	infos, err := ctx.Devices(malgo.Playback)
	if err != nil {
		t.Fatal(err)
	}
	for _, info := range infos {
		id, err := malgo.ParseDeviceID(info.ID.String())
		if err != nil {
			t.Fatal(err)
		}
		if id != info.ID {
			t.Fatalf("parsed %v, want %v", id, info.ID)
		}
	}

	id, err := malgo.ParseDeviceID("0102ff")
	if err != nil {
		t.Fatal(err)
	}
	if id[0] != 1 || id[1] != 2 || id[2] != 0xff || id[3] != 0 {
		t.Fatalf("parsed %v", id[:4])
	}

	for _, s := range []string{"zz", "123", strings.Repeat("00", len(malgo.DeviceID{})+1)} {
		if _, err := malgo.ParseDeviceID(s); err == nil {
			t.Fatalf("parsed invalid id %q", s)
		}
	}
}

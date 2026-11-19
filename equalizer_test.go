package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"testing"
)

// toneSample is a sine tone at amplitude 0.25, as the decoder would queue it.
func toneSample(frequency float64, frame int) float32 {
	return float32(0.25 * math.Sin(2*math.Pi*frequency*float64(frame)/outputSampleRate))
}

// playingTestPlayer returns a player that is playing a ring full of a tone,
// as if the decoder had already queued the next two seconds.
func playingTestPlayer(t *testing.T, frequency float64) *AudioPlayer {
	t.Helper()
	player := newTestPlayer(&fakeOutputs{})
	player.volume = 1
	player.playing = true
	frames := player.ring.capacity
	samples := make([]float32, frames*outputChannels)
	for i := 0; i < frames; i++ {
		samples[i*2] = toneSample(frequency, i)
		samples[i*2+1] = samples[i*2]
	}
	if queued := player.ring.push(0, samples, frames); queued != frames {
		t.Fatalf("queued %d frames, want %d", queued, frames)
	}
	return player
}

// renderLeft runs the device callback for frames frames and returns the left
// channel it wrote.
func renderLeft(player *AudioPlayer, frames int) []float64 {
	output := make([]byte, frames*outputChannels*4)
	player.render(output, nil, uint32(frames))
	left := make([]float64, frames)
	for i := range left {
		left[i] = float64(math.Float32frombits(binary.LittleEndian.Uint32(output[i*8 : i*8+4])))
	}
	return left
}

func rms(samples []float64) float64 {
	sum := 0.0
	for _, sample := range samples {
		sum += sample * sample
	}
	return math.Sqrt(sum / float64(len(samples)))
}

// tenBands returns equalizer settings with gainDB on the band at index.
func tenBands(index int, gainDB float64) []float64 {
	bands := make([]float64, 10)
	bands[index] = gainDB
	return bands
}

const tenthOfASecond = outputSampleRate / 10

func TestEqualizerChangeIsHeardRightAway(t *testing.T) {
	player := playingTestPlayer(t, 1000)
	before := rms(renderLeft(player, tenthOfASecond))

	player.SetEqualizer(true, tenBands(5, -12)) // the 1 kHz band
	renderLeft(player, tenthOfASecond)          // the change glides in here
	after := rms(renderLeft(player, tenthOfASecond))

	// -12 dB is a quarter of the level. Before the change, two seconds of
	// already equalized audio were queued ahead of the speakers.
	if after > before*0.3 {
		t.Fatalf("1 kHz tone level went from %.3f to %.3f within 0.2 s of cutting its band by 12 dB; want about a quarter", before, after)
	}
}

func TestEqualizerGlidesInsteadOfJumping(t *testing.T) {
	player := playingTestPlayer(t, 1000)
	before := rms(renderLeft(player, tenthOfASecond))

	player.SetEqualizer(true, tenBands(5, -12))

	// The first 5 ms still sound close to the old level: a 12 dB jump clicks.
	if first := rms(renderLeft(player, outputSampleRate/200)); first < before*0.6 {
		t.Fatalf("level fell from %.3f to %.3f in the first 5 ms; want a glide", before, first)
	}
}

// assertUntouched renders 10 ms and checks every sample is the queued tone
// times the volume, bit for bit.
func assertUntouched(t *testing.T, player *AudioPlayer, frequency float64) {
	t.Helper()
	start := int(player.positionFrames)
	for i, sample := range renderLeft(player, outputSampleRate/100) {
		want := float64(toFloat32(float64(toneSample(frequency, start+i)) * player.volume))
		if sample != want {
			t.Fatalf("sample %d = %v; want the original %v", i, sample, want)
		}
	}
}

func TestEqualizerOffLeavesTheSoundUntouched(t *testing.T) {
	player := playingTestPlayer(t, 1000)
	player.volume = 0.5
	player.SetEqualizer(true, tenBands(0, 6))
	renderLeft(player, tenthOfASecond)

	player.SetEqualizer(false, tenBands(0, 6))
	renderLeft(player, tenthOfASecond*3) // glides back to flat

	assertUntouched(t, player, 1000)
}

func TestFlatEqualizerLeavesTheSoundUntouched(t *testing.T) {
	player := playingTestPlayer(t, 1000)
	player.SetEqualizer(true, make([]float64, 10))

	assertUntouched(t, player, 1000)
}

func TestEqualizerHasTenBandsWithinRange(t *testing.T) {
	player := newTestPlayer(&fakeOutputs{})

	player.SetEqualizer(true, []float64{20, -20, 1, 2, 3, 4, 5, 6, 7, 8})

	want := []float64{12, -12, 1, 2, 3, 4, 5, 6, 7, 8}
	if got := player.State().Equalizer; !slicesEqual(got, want) {
		t.Fatalf("equalizer = %v; want %v", got, want)
	}
}

func slicesEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestEqualizerBoostLeavesHeadroom(t *testing.T) {
	var bands [eqBandCount]float64
	for i := range bands {
		bands[i] = 12
	}
	for _, frequency := range []float64{31.25, 125, 1000, 4000, 16000} {
		if peak := peakAfterEQ(bands, frequency, 1); peak > 1 {
			t.Errorf("a full-scale %v Hz tone peaks at %.3f with every band at +12 dB; want at most 1", frequency, peak)
		}
	}
}

func TestEqualizerCutKeepsLevelAwayFromItsBand(t *testing.T) {
	var bands [eqBandCount]float64
	bands[0] = -6 // 31 Hz

	// 10 kHz, unlike 8 kHz, has samples on the crest of the wave at 48 kHz.
	if peak := peakAfterEQ(bands, 10000, 0.5); math.Abs(peak-0.5) > 0.01 {
		t.Fatalf("a 10 kHz tone at 0.5 came out at %.3f with only the deepest bass cut", peak)
	}
}

// peakAfterEQ plays one second of a sine tone through an equalizer set to
// bands and returns the largest sample of the last half second, once the
// settings have glided in and the filters have settled.
func peakAfterEQ(bands [eqBandCount]float64, frequency, amplitude float64) float64 {
	var eq eqProcessor
	eq.setTarget(true, bands, eqPreampDB(bands))
	samples := make([]float32, outputSampleRate*outputChannels)
	for i := 0; i < outputSampleRate; i++ {
		sample := float32(amplitude * math.Sin(2*math.Pi*frequency*float64(i)/outputSampleRate))
		samples[i*2], samples[i*2+1] = sample, sample
	}
	eq.process(samples)
	peak := 0.0
	for i := outputSampleRate / 2; i < outputSampleRate; i++ {
		peak = math.Max(peak, math.Abs(float64(samples[i*2])))
	}
	return peak
}

func TestEqualizerSettingsAreRemembered(t *testing.T) {
	store := openTestStore(t)
	service := NewLibraryService(store, newTestPlayer(&fakeOutputs{}))
	bands := []float64{1, 2, 3, 4, 5, -6, -5, -4, -3, -2.5}
	service.SetEqualizer(true, bands)

	reopened := NewLibraryService(store, newTestPlayer(&fakeOutputs{}))
	reopened.loadEqualizer(context.Background())

	if state := reopened.player.State(); !state.EqualizerOn || !slicesEqual(state.Equalizer, bands) {
		t.Fatalf("after reopening: on=%v bands=%v; want on with %v", state.EqualizerOn, state.Equalizer, bands)
	}
}

func TestThreeBandSettingsMoveToTenBandsOnStartup(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.SetPreference(ctx, "equalizerEnabled", "true"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetPreference(ctx, "equalizerBands", "[6,0,0]"); err != nil {
		t.Fatal(err)
	}
	service := NewLibraryService(store, newTestPlayer(&fakeOutputs{}))

	service.loadEqualizer(ctx)

	state := service.player.State()
	if !state.EqualizerOn || len(state.Equalizer) != 10 || state.Equalizer[1] < 4 || state.Equalizer[0] > 6 {
		t.Fatalf("equalizer after upgrade: on=%v %v; want the old bass boost spread over the ten bands", state.EqualizerOn, state.Equalizer)
	}
	raw, _ := store.Preference(ctx, "equalizerBands", "")
	var saved []float64
	if err := json.Unmarshal([]byte(raw), &saved); err != nil || !slicesEqual(saved, state.Equalizer) {
		t.Fatalf("saved bands = %s; want the ten converted bands %v", raw, state.Equalizer)
	}
}

func TestThreeBandSettingsBecomeTenBands(t *testing.T) {
	bands := legacyEqualizerBands([3]float64{6, 0, 0}) // the old bass boost

	if bands[1] < 4 || bands[2] < 4 {
		t.Errorf("62 and 125 Hz = %v, %v dB; want the bass boost kept", bands[1], bands[2])
	}
	if math.Abs(bands[8]) > 0.5 || math.Abs(bands[9]) > 0.5 {
		t.Errorf("8 and 16 kHz = %v, %v dB; want them about flat", bands[8], bands[9])
	}
	for i, gain := range bands {
		if gain != math.Round(gain*2)/2 {
			t.Errorf("band %d = %v dB; want half-decibel steps like the sliders", i, gain)
		}
	}
}

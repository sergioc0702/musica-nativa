package main

import (
	"math"
	"math/cmplx"
)

// The equalizer has ten bands an octave apart, like a graphic equalizer.
// Each band is a peaking filter about one octave wide.
const (
	eqBandCount = 10
	eqBandQ     = 1.41
	eqMaxGainDB = 12
	// eqBlockFrames is how often a setting that is still changing moves on.
	eqBlockFrames = 64
	// eqGlideDB is how far a band moves every eqBlockFrames frames (1.3 ms):
	// a change is heard right away, but as a quick glide rather than a jump,
	// which would click. A 12 dB change takes about 50 ms.
	eqGlideDB = 0.3
	// eqHeadroomMarginDB covers a peak that falls between the frequencies checked.
	eqHeadroomMarginDB = 0.1
)

// eqFrequencies are the centers of the bands, in hertz.
var eqFrequencies = [eqBandCount]float64{31.25, 62.5, 125, 250, 500, 1000, 2000, 4000, 8000, 16000}

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

// setShape takes the coefficients of shape and keeps the filter's memory,
// so the sound carries on without a break.
func (b *biquad) setShape(shape biquad) {
	b.b0, b.b1, b.b2, b.a1, b.a2 = shape.b0, shape.b1, shape.b2, shape.a1, shape.a2
}

// magnitude returns the filter's gain at angular frequency w (radians/sample).
func (b biquad) magnitude(w float64) float64 {
	z1 := cmplx.Exp(complex(0, -w))
	z2 := z1 * z1
	numerator := complex(b.b0, 0) + complex(b.b1, 0)*z1 + complex(b.b2, 0)*z2
	denominator := 1 + complex(b.a1, 0)*z1 + complex(b.a2, 0)*z2
	return cmplx.Abs(numerator / denominator)
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

// eqBands reads settings for the ten bands, limited to ±12 dB. Bands that
// are missing stay flat.
func eqBands(values []float64) [eqBandCount]float64 {
	var bands [eqBandCount]float64
	for i := 0; i < eqBandCount && i < len(values); i++ {
		if value := values[i]; !math.IsNaN(value) {
			bands[i] = math.Max(-eqMaxGainDB, math.Min(eqMaxGainDB, value))
		}
	}
	return bands
}

// eqPreampDB is how far to lower the input so the largest boost of bands
// still fits below full scale instead of clipping.
func eqPreampDB(bands [eqBandCount]float64) float64 {
	filters := make([]biquad, eqBandCount)
	for band, gain := range bands {
		filters[band] = makePeakingEQ(outputSampleRate, eqFrequencies[band], gain, eqBandQ)
	}
	if peak := cascadePeakDB(filters, outputSampleRate); peak > 0.01 {
		return -(peak + eqHeadroomMarginDB)
	}
	return 0
}

// legacyEqualizerBands turns the three bands of version 1.0.2 and earlier
// (100 Hz, 1 kHz and 10 kHz) into the ten bands, keeping the curve the
// listener had at each band's center.
func legacyEqualizerBands(old [3]float64) [eqBandCount]float64 {
	var filters [3]biquad
	for i, frequency := range [3]float64{100, 1000, 10000} {
		filters[i] = makePeakingEQ(outputSampleRate, frequency, old[i], 0.707)
	}
	var bands [eqBandCount]float64
	for band, frequency := range eqFrequencies {
		w := 2 * math.Pi * frequency / outputSampleRate
		gain := 1.0
		for _, filter := range filters {
			gain *= filter.magnitude(w)
		}
		decibels := math.Round(40*math.Log10(gain)) / 2 // half-decibel steps
		bands[band] = math.Max(-eqMaxGainDB, math.Min(eqMaxGainDB, decibels))
	}
	return bands
}

func decibelsToGain(decibels float64) float64 { return math.Pow(10, decibels/20) }

// approach moves value toward target by at most step.
func approach(value, target, step float64) float64 {
	switch {
	case math.Abs(target-value) <= step:
		return target
	case target > value:
		return value + step
	default:
		return value - step
	}
}

// eqProcessor equalizes the audio on its way to the speakers. Its settings
// glide from where they are toward the target, and while every band is flat
// it leaves the samples untouched. The zero value is a flat equalizer.
type eqProcessor struct {
	target  [eqBandCount]float64 // dB
	current [eqBandCount]float64 // dB, on its way to target
	// The input is lowered by the preamp (dB, at most 0) so that boosts
	// keep their shape instead of clipping at full scale.
	targetPreamp float64
	preamp       float64
	filters      [outputChannels][eqBandCount]biquad
	// shaping is true while the filters are in use and hold their memory.
	shaping bool
}

// setTarget starts moving toward bands (dB) with the input lowered by
// preampDB. Turning the equalizer off glides every band back to flat.
func (e *eqProcessor) setTarget(enabled bool, bands [eqBandCount]float64, preampDB float64) {
	if !enabled {
		bands, preampDB = [eqBandCount]float64{}, 0
	}
	e.target, e.targetPreamp = bands, preampDB
}

// flat reports whether the equalizer leaves the sound alone now and will go on doing so.
func (e *eqProcessor) flat() bool {
	var none [eqBandCount]float64
	return e.current == none && e.target == none && e.preamp == 0 && e.targetPreamp == 0
}

// process equalizes interleaved stereo samples in place.
func (e *eqProcessor) process(samples []float32) {
	if e.flat() {
		e.shaping = false
		return
	}
	if !e.shaping {
		// Every band is flat here, so the filters start out passing the sound through.
		for channel := range e.filters {
			for band := range e.filters[channel] {
				e.filters[channel][band] = biquad{b0: 1}
			}
		}
		e.shaping = true
	}
	frames := len(samples) / outputChannels
	for start := 0; start < frames; start += eqBlockFrames {
		end := min(start+eqBlockFrames, frames)
		from := decibelsToGain(e.preamp)
		e.glide()
		step := (decibelsToGain(e.preamp) - from) / float64(end-start)
		for frame := start; frame < end; frame++ {
			gain := from + step*float64(frame-start+1)
			for channel := range e.filters {
				index := frame*outputChannels + channel
				sample := float64(samples[index]) * gain
				for band := range e.filters[channel] {
					sample = e.filters[channel][band].process(sample)
				}
				samples[index] = float32(sample)
			}
		}
	}
}

// glide moves every setting one step toward its target.
func (e *eqProcessor) glide() {
	for band := range e.current {
		if e.current[band] == e.target[band] {
			continue
		}
		e.current[band] = approach(e.current[band], e.target[band], eqGlideDB)
		shape := makePeakingEQ(outputSampleRate, eqFrequencies[band], e.current[band], eqBandQ)
		for channel := range e.filters {
			e.filters[channel][band].setShape(shape)
		}
	}
	// The input goes down faster than a band goes up, so a boost never clips
	// while it glides in.
	step := eqGlideDB
	if e.targetPreamp < e.preamp {
		step *= 3
	}
	e.preamp = approach(e.preamp, e.targetPreamp, step)
}

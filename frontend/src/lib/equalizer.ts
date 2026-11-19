// The ten bands of the equalizer, as the audio engine has them (equalizer.go).
export const eqBands = [
  { frequency: 31.25, label: '31' },
  { frequency: 62.5, label: '62' },
  { frequency: 125, label: '125' },
  { frequency: 250, label: '250' },
  { frequency: 500, label: '500' },
  { frequency: 1000, label: '1k' },
  { frequency: 2000, label: '2k' },
  { frequency: 4000, label: '4k' },
  { frequency: 8000, label: '8k' },
  { frequency: 16000, label: '16k' },
]

export const eqMaxGain = 12
const sampleRate = 48000
const bandQ = 1.41

export type EqPreset = { id: string; name: string; bands: number[] }

export const eqPresets: EqPreset[] = [
  { id: 'flat', name: 'Plano', bands: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0] },
  { id: 'bass', name: 'Graves', bands: [6, 5.5, 4.5, 3, 1, 0, 0, 0, 0, 0] },
  { id: 'treble', name: 'Agudos', bands: [0, 0, 0, 0, 0, 0.5, 1.5, 3, 4.5, 5.5] },
  { id: 'vocal', name: 'Voz', bands: [-2, -2, -1, 0, 1.5, 3, 3.5, 2.5, 1, 0] },
  { id: 'rock', name: 'Rock', bands: [4.5, 3.5, 2, 0.5, -1, -1, 0.5, 2, 3.5, 4.5] },
  { id: 'pop', name: 'Pop', bands: [-1, 0, 1.5, 3, 3.5, 2.5, 1, 0, -0.5, -1] },
  { id: 'electronic', name: 'Electrónica', bands: [5, 4.5, 2.5, 0, -1.5, -0.5, 1, 2.5, 3.5, 4] },
  { id: 'jazz', name: 'Jazz', bands: [3, 2, 1, 1.5, -1, -1, 0, 1, 2, 3] },
  { id: 'classical', name: 'Clásica', bands: [3.5, 2.5, 1.5, 1, -0.5, -0.5, 0, 1.5, 2.5, 3] },
  { id: 'acoustic', name: 'Acústico', bands: [3, 2.5, 2, 1, 1.5, 1.5, 2, 2.5, 2, 1.5] },
]

// presetFor returns the preset that bands match, or undefined for custom settings.
export function presetFor(bands: number[]): EqPreset | undefined {
  return eqPresets.find((preset) => preset.bands.every((gain, index) => gain === (bands[index] ?? 0)))
}

// snapGain keeps a slider value within ±12 dB, in half-decibel steps.
export function snapGain(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.max(-eqMaxGain, Math.min(eqMaxGain, Math.round(value * 2) / 2))
}

// peakingGain is the gain of one band at frequency: the same peaking filter
// the audio engine uses, so the curve shows what you hear.
function peakingGain(center: number, gainDB: number, frequency: number): number {
  if (gainDB === 0) return 1
  const a = 10 ** (gainDB / 40)
  const w0 = (2 * Math.PI * center) / sampleRate
  const alpha = Math.sin(w0) / (2 * bandQ)
  const cosine = Math.cos(w0)
  const a0 = 1 + alpha / a
  const [b0, b1, b2] = [(1 + alpha * a) / a0, (-2 * cosine) / a0, (1 - alpha * a) / a0]
  const [a1, a2] = [(-2 * cosine) / a0, (1 - alpha / a) / a0]
  const w = (2 * Math.PI * frequency) / sampleRate
  const size = (c0: number, c1: number, c2: number) =>
    Math.hypot(c0 + c1 * Math.cos(w) + c2 * Math.cos(2 * w), c1 * Math.sin(w) + c2 * Math.sin(2 * w))
  return size(b0, b1, b2) / size(1, a1, a2)
}

// responseDB is the gain of all the bands together at frequency, in decibels.
export function responseDB(bands: number[], frequency: number): number {
  let gain = 1
  eqBands.forEach((band, index) => { gain *= peakingGain(band.frequency, bands[index] ?? 0, frequency) })
  return 20 * Math.log10(gain)
}

// The curve spans half an octave beyond the outer bands, so each band's
// center sits right above its slider when the sliders share the width evenly.
const curveLow = eqBands[0].frequency / Math.SQRT2
const curveHigh = eqBands[eqBands.length - 1].frequency * Math.SQRT2

// curvePath draws responseDB as an SVG path in a width × height box, with
// 0 dB across the middle and ±rangeDB at the edges.
export function curvePath(bands: number[], width: number, height: number, rangeDB = 15, points = 96): string {
  const commands: string[] = []
  for (let i = 0; i <= points; i++) {
    const frequency = curveLow * (curveHigh / curveLow) ** (i / points)
    const decibels = Math.max(-rangeDB, Math.min(rangeDB, responseDB(bands, frequency)))
    const x = (i / points) * width
    const y = height / 2 - (decibels / rangeDB) * (height / 2)
    commands.push(`${i === 0 ? 'M' : 'L'}${x.toFixed(1)} ${y.toFixed(1)}`)
  }
  return commands.join(' ')
}

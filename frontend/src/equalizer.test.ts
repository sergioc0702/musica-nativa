// Run with `npm test` (Node's own test runner; no extra packages).
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { curvePath, eqBands, eqPresets, presetFor, responseDB, snapGain } from './equalizer.ts'

const flat = () => eqBands.map(() => 0)
const withBand = (index: number, gain: number) => flat().map((value, i) => (i === index ? gain : value))

test('a flat equalizer does not change any frequency', () => {
  for (const frequency of [25, 100, 1000, 10000, 20000]) {
    assert.equal(responseDB(flat(), frequency), 0)
  }
})

test('a band boosts its own frequency and leaves far ones alone', () => {
  const bands = withBand(5, 6) // 1 kHz
  assert.ok(Math.abs(responseDB(bands, 1000) - 6) < 0.05, `1 kHz = ${responseDB(bands, 1000)} dB`)
  assert.ok(Math.abs(responseDB(bands, 31.25)) < 0.2, `31 Hz = ${responseDB(bands, 31.25)} dB`)
})

test('the curve peaks right above the slider of the boosted band', () => {
  const width = 1000
  for (const index of [0, 5, 9]) {
    const points = [...curvePath(withBand(index, 9), width, 100).matchAll(/[ML](\S+) (\S+)/g)]
      .map((match) => ({ x: Number(match[1]), y: Number(match[2]) }))
    const top = points.reduce((best, point) => (point.y < best.y ? point : best))
    const sliderX = ((index + 0.5) / eqBands.length) * width
    assert.ok(Math.abs(top.x - sliderX) <= 12, `band ${index}: peak at x=${top.x}, slider at x=${sliderX}`)
  }
})

test('a flat curve is a straight line through the middle', () => {
  const ys = [...curvePath(flat(), 200, 80).matchAll(/[ML]\S+ (\S+)/g)].map((match) => Number(match[1]))
  assert.ok(ys.every((y) => y === 40), `y values: ${[...new Set(ys)]}`)
})

test('every preset sets the ten bands within ±12 dB in half-decibel steps', () => {
  assert.equal(new Set(eqPresets.map((preset) => preset.id)).size, eqPresets.length)
  for (const preset of eqPresets) {
    assert.equal(preset.bands.length, eqBands.length, preset.name)
    for (const gain of preset.bands) assert.equal(snapGain(gain), gain, `${preset.name}: ${gain}`)
  }
})

test('settings are named after the preset they match, or are custom', () => {
  assert.equal(presetFor(flat())?.name, 'Plano')
  const rock = eqPresets.find((preset) => preset.id === 'rock')!
  assert.equal(presetFor([...rock.bands])?.id, 'rock')
  assert.equal(presetFor(withBand(3, 0.5)), undefined)
})

test('slider values stay within ±12 dB in half-decibel steps', () => {
  assert.equal(snapGain(12.7), 12)
  assert.equal(snapGain(-30), -12)
  assert.equal(snapGain(-3.26), -3.5)
  assert.equal(snapGain(Number.NaN), 0)
})

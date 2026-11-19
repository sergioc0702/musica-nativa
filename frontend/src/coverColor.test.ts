// Run with `npm test` (Node's own test runner; no extra packages).
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { vividColor } from './coverColor.ts'

// pixels builds RGBA data from [r, g, b, count] runs.
function pixels(...runs: [number, number, number, number][]): Uint8ClampedArray {
  const data: number[] = []
  for (const [r, g, b, count] of runs) for (let i = 0; i < count; i++) data.push(r, g, b, 255)
  return new Uint8ClampedArray(data)
}

const channels = (color: string) => color.match(/\d+/g)!.map(Number)

test('a cover in one color gives that color back', () => {
  const [r, g, b] = channels(vividColor(pixels([214, 98, 52, 100]))!)
  assert.ok(r > g && g > b, `rgb(${r}, ${g}, ${b}) is not orange`)
})

test('the vivid part of a mostly gray cover wins', () => {
  const [r, g, b] = channels(vividColor(pixels([120, 120, 122, 90], [40, 90, 220, 10]))!)
  assert.ok(b > r && b > g, `rgb(${r}, ${g}, ${b}) is not blue`)
})

test('black, white and gray covers have no color to take', () => {
  assert.equal(vividColor(pixels([0, 0, 0, 50], [255, 255, 255, 50])), null)
  assert.equal(vividColor(pixels([128, 128, 128, 100])), null)
})

test('the color stays light enough to tint a dark page', () => {
  const [r, g, b] = channels(vividColor(pixels([60, 10, 20, 100]))!)
  assert.ok(Math.max(r, g, b) >= 110, `rgb(${r}, ${g}, ${b}) is too dark to show`)
})

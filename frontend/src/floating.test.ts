// Run with `npm test` (Node's own test runner; no extra packages).
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { clampRect, togglePanel, raisePanel } from './floating.ts'

const window = { width: 1280, height: 760 }
const min = { width: 300, height: 200 }

test('a panel inside the window stays where it is', () => {
  assert.deepEqual(clampRect({ x: 100, y: 80, width: 400, height: 300 }, window, min), { x: 100, y: 80, width: 400, height: 300 })
})

test('a panel dragged off the edge comes back so its bar can be grabbed', () => {
  const rect = clampRect({ x: 1500, y: -40, width: 400, height: 300 }, window, min)
  assert.equal(rect.y, 0)
  assert.ok(rect.x + 120 <= window.width, `x = ${rect.x} leaves the bar out of reach`)
  assert.equal(clampRect({ x: -900, y: 900, width: 400, height: 300 }, window, min).x, -280)
  assert.ok(clampRect({ x: 0, y: 900, width: 400, height: 300 }, window, min).y <= window.height - 44)
})

test('a panel never gets smaller than its minimum or larger than the window', () => {
  assert.deepEqual(clampRect({ x: 0, y: 0, width: 50, height: 20 }, window, min), { x: 0, y: 0, width: 300, height: 200 })
  const huge = clampRect({ x: 0, y: 0, width: 5000, height: 5000 }, window, min)
  assert.equal(huge.width, window.width)
  assert.equal(huge.height, window.height)
})

test('opening a panel puts it on top; opening it again closes it', () => {
  assert.deepEqual(togglePanel(['queue'], 'lyrics'), ['queue', 'lyrics'])
  assert.deepEqual(togglePanel(['queue', 'lyrics'], 'queue'), ['lyrics'])
})

test('clicking a panel brings it to the front', () => {
  assert.deepEqual(raisePanel(['equalizer', 'queue', 'lyrics'], 'equalizer'), ['queue', 'lyrics', 'equalizer'])
  assert.deepEqual(raisePanel(['queue'], 'lyrics'), ['queue'])
})

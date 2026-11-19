// Run with `npm test` (Node's own test runner; no extra packages).
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { currentLineIndex, filledLetters } from './lyrics.ts'

const lines = [
  { timeMs: 1000, text: 'Hola mundo' },
  { timeMs: 3000, text: 'Segunda línea' },
  { timeMs: 60000, text: 'Tras el solo' },
]

test('the current line is the last one that has started', () => {
  assert.equal(currentLineIndex(lines, 500), -1)
  assert.equal(currentLineIndex(lines, 1000), 0)
  assert.equal(currentLineIndex(lines, 2999), 0)
  assert.equal(currentLineIndex(lines, 3000), 1)
  assert.equal(currentLineIndex(lines, 90000), 2)
})

test('a line fills letter by letter while it is sung', () => {
  // "Hola mundo" is sung in 1.7 s: about 9 letters a second plus a moment.
  assert.equal(filledLetters(lines[0], 3000, 900), 0)
  assert.equal(filledLetters(lines[0], 3000, 1850), 5)
  assert.equal(filledLetters(lines[0], 3000, 2800), 10)
})

test('a line before a long instrumental fills at singing speed', () => {
  // The next line starts 57 s later; this one is still sung in about 2 s.
  assert.equal(filledLetters(lines[1], 60000, 5500), lines[1].text.length)
})

test('word timings fill each word in its own time', () => {
  const line = { timeMs: 1000, text: 'Hola mundo', words: [{ timeMs: 1000, text: 'Hola ' }, { timeMs: 1800, text: 'mundo' }] }
  assert.equal(filledLetters(line, 4000, 1400), 3) // halfway through "Hola "
  assert.equal(filledLetters(line, 4000, 1799), 5)
  assert.equal(filledLetters(line, 4000, 2000), 7) // "mundo" is sung in 0.55 s
  assert.equal(filledLetters(line, 4000, 3000), 10)
})

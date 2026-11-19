// Run with `npm test` (Node's own test runner; no extra packages).
import { test } from 'node:test'
import assert from 'node:assert/strict'
import type { Track } from '../bindings/musica-nativa/internal/library/models'
import { formatLength, sortTracks } from './trackSort.ts'

const song = (title: string, artist: string, album: string, trackNumber: number, durationSeconds: number, modifiedUnix: number) =>
  ({ id: title, path: `${title}.mp3`, title, artist, album, albumArtist: artist, trackNumber, discNumber: 0, durationSeconds, modifiedUnix }) as Track

const library = [
  song('Volver', 'Luna Verde', 'Horizonte', 4, 240, 10),
  song('Ángel', 'Los Ríos', 'Mar adentro', 2, 180, 40),
  song('amanecer', 'Luna Verde', 'Horizonte', 1, 200, 20),
  song('Canción 10', 'Cielo Norte', 'Ruta 40', 2, 90, 30),
  song('Canción 9', 'Cielo Norte', 'Ruta 40', 1, 300, 50),
]
const titles = (tracks: Track[]) => tracks.map((track) => track.title)

test('by title, accents and capitals do not matter and numbers read as numbers', () => {
  assert.deepEqual(titles(sortTracks(library, 'title', false)), ['amanecer', 'Ángel', 'Canción 9', 'Canción 10', 'Volver'])
})

test('by album, each album keeps the order of the record', () => {
  assert.deepEqual(titles(sortTracks(library, 'album', false)), ['amanecer', 'Volver', 'Ángel', 'Canción 9', 'Canción 10'])
})

test('by artist, then by album in record order', () => {
  assert.deepEqual(titles(sortTracks(library, 'artist', false)), ['Canción 9', 'Canción 10', 'Ángel', 'amanecer', 'Volver'])
})

test('by length, shortest first, and descending reverses it', () => {
  assert.deepEqual(titles(sortTracks(library, 'duration', false)), ['Canción 10', 'Ángel', 'amanecer', 'Volver', 'Canción 9'])
  assert.deepEqual(titles(sortTracks(library, 'duration', true)), ['Canción 9', 'Volver', 'amanecer', 'Ángel', 'Canción 10'])
})

test('recent first puts the newest files on top', () => {
  assert.deepEqual(titles(sortTracks(library, 'added', false)), ['Canción 9', 'Ángel', 'Canción 10', 'amanecer', 'Volver'])
})

test('the given order stays as it is, and the list itself is not changed', () => {
  const copy = [...library]
  assert.deepEqual(titles(sortTracks(library, 'order', false)), titles(copy))
  sortTracks(library, 'title', false)
  assert.deepEqual(library, copy)
})

test('a total length reads in hours and minutes', () => {
  assert.equal(formatLength(45 * 60 + 20), '45 min')
  assert.equal(formatLength(3600 + 12 * 60), '1 h 12 min')
  assert.equal(formatLength(2 * 3600), '2 h')
  assert.equal(formatLength(0), '0 min')
})

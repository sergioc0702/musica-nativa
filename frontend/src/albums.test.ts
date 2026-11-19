// Run with `npm test` (Node's own test runner; no extra packages).
import { test } from 'node:test'
import assert from 'node:assert/strict'
import type { Track } from '../bindings/musica-nativa/internal/library/models'
import { groupAlbums } from './albums.ts'

const song = (title: string, trackNumber: number, discNumber = 0, path = `${title}.mp3`) =>
  ({ id: title, path, title, artist: 'Luna Verde', album: 'Horizonte', albumArtist: 'Luna Verde', trackNumber, discNumber }) as Track

const titles = (tracks: Track[]) => tracks.map((track) => track.title)

test('an album plays in track order, not by title', () => {
  // The library arrives sorted by title.
  const [album] = groupAlbums([song('Amanecer', 1), song('Cielo abierto', 3), song('Ruta al sur', 2), song('Volver', 4)])

  assert.deepEqual(titles(album.tracks), ['Amanecer', 'Ruta al sur', 'Cielo abierto', 'Volver'])
})

test('a double album plays disc 1 before disc 2', () => {
  const [album] = groupAlbums([song('Bis', 1, 2), song('Coda', 2, 2), song('Inicio', 1), song('Puente', 2, 1)])

  assert.deepEqual(titles(album.tracks), ['Inicio', 'Puente', 'Bis', 'Coda'])
})

test('songs without a track number go last, by file name', () => {
  const [album] = groupAlbums([song('Bonus', 0, 0, '10 Bonus.mp3'), song('Extra', 0, 0, '9 Extra.mp3'), song('Tema', 1)])

  assert.deepEqual(titles(album.tracks), ['Tema', 'Extra', 'Bonus'])
})

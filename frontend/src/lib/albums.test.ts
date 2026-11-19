// Run with `npm test` (Node's own test runner; no extra packages).
import { test } from 'node:test'
import assert from 'node:assert/strict'
import type { Track } from '../../bindings/musica-nativa/internal/library/models'
import { albumSeconds, albumYear, groupAlbums, sortAlbums, type AlbumSort } from './albums.ts'

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

test('albums sort by name, artist, newest year or newest files', () => {
  const tracks = [
    { ...song('a', 1), album: 'Zeta', albumArtist: 'Banda', year: 1999, modifiedUnix: 30 },
    { ...song('b', 1), album: 'Álamo', albumArtist: 'Coro', year: 2021, modifiedUnix: 10 },
    { ...song('c', 1), album: 'Mar', albumArtist: 'Aria', year: 0, modifiedUnix: 20 },
  ] as Track[]
  const names = (key: AlbumSort) => sortAlbums(groupAlbums(tracks), key).map((album) => album.name)
  assert.deepEqual(names('name'), ['Álamo', 'Mar', 'Zeta'])
  assert.deepEqual(names('artist'), ['Mar', 'Zeta', 'Álamo'])
  assert.deepEqual(names('year'), ['Álamo', 'Zeta', 'Mar']) // unknown years last
  assert.deepEqual(names('added'), ['Zeta', 'Mar', 'Álamo'])
})

test('an album shows its year and total length', () => {
  const [album] = groupAlbums([{ ...song('a', 1), year: 2004, durationSeconds: 200 }, { ...song('b', 2), year: 0, durationSeconds: 100 }] as Track[])
  assert.equal(albumYear(album), 2004)
  assert.equal(albumSeconds(album), 300)
})

test('an album whose artist is written with different capitals stays one album', () => {
  const tracks = [
    { ...song('Uno', 1), album: 'Day69', albumArtist: '6ix9ine' },
    { ...song('Dos', 2), album: 'DAY69', albumArtist: '6IX9INE' },
  ] as Track[]
  const albums = groupAlbums(tracks)
  assert.equal(albums.length, 1)
  assert.deepEqual(titles(albums[0].tracks), ['Uno', 'Dos'])
})

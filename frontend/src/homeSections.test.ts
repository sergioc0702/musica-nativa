// Run with `npm test` (Node's own test runner; no extra packages).
import { test } from 'node:test'
import assert from 'node:assert/strict'
import type { Track } from '../bindings/musica-nativa/internal/library/models'
import { groupAlbums, nameKey } from './albums.ts'
import { artistAlbums, artistInitials, dailyMix, greeting, libraryStats, recentAlbums, rediscover, sortArtists, topArtists, topTracks } from './homeSections.ts'

const song = (id: string, album: string, artist: string, durationSeconds = 200, available = true) =>
  ({ id, path: `${id}.mp3`, title: id, artist, album, albumArtist: artist, trackNumber: 1, discNumber: 0, durationSeconds, available }) as Track

const library = [
  song('a1', 'Horizonte', 'Luna Verde'), song('a2', 'Horizonte', 'Luna Verde'),
  song('b1', 'Mar adentro', 'Los Ríos'), song('c1', 'Ruta 40', 'Cielo Norte'),
  song('d1', 'Noches', 'Marea Alta'), song('x1', 'Perdida', 'Nadie', 200, false),
]
const albums = groupAlbums(library)
const day = 86400

test('the greeting follows the time of day', () => {
  assert.equal(greeting(7), 'Buenos días')
  assert.equal(greeting(13), 'Buenas tardes')
  assert.equal(greeting(21), 'Buenas noches')
  assert.equal(greeting(3), 'Buenas noches')
})

test('the summary counts songs, albums, artists and listening time', () => {
  assert.deepEqual(libraryStats(library), { songs: 6, albums: 5, artists: 5, time: '20 min' })
  assert.equal(libraryStats([song('l', 'L', 'L', 3 * 3600 + 100)]).time, '3 h')
})

test('the mix of the day stays the same all day and changes the next', () => {
  const big = Array.from({ length: 60 }, (_, i) => song(`s${i}`, `A${i % 7}`, `R${i % 5}`))
  const today = dailyMix(big, '2026-11-19').map((track) => track.id)
  assert.equal(today.length, 25)
  assert.deepEqual(dailyMix(big, '2026-11-19').map((track) => track.id), today)
  assert.notDeepEqual(dailyMix(big, '2026-11-20').map((track) => track.id), today)
  assert.ok(!dailyMix(library, '2026-11-19').some((track) => !track.available), 'a missing file got into the mix')
})

test('recent albums come from the last songs listened, each album once', () => {
  const recent = recentAlbums(['a2', 'b1', 'a1', 'missing', 'c1'], library, albums)
  assert.deepEqual(recent.map((album) => album.name), ['Horizonte', 'Mar adentro', 'Ruta 40'])
})

test('top songs keep their play counts and skip songs no longer in the library', () => {
  const top = topTracks([{ trackId: 'b1', plays: 9 }, { trackId: 'gone', plays: 5 }, { trackId: 'a1', plays: 2 }], library)
  assert.deepEqual(top.map(({ track, plays }) => [track.id, plays]), [['b1', 9], ['a1', 2]])
})

test('rediscover leaves out albums heard in the last month', () => {
  const now = 1_800_000_000
  const lastPlayed = { a1: now - 2 * day, b1: now - 90 * day }
  const names = rediscover(albums, lastPlayed, now, 1).map((album) => album.name)
  assert.ok(!names.includes('Horizonte'), 'an album heard two days ago came back')
  assert.ok(names.includes('Mar adentro'), 'an album not heard for three months is missing')
  assert.ok(!names.includes('Perdida'), 'an album whose files are missing was offered')
})

test('artists listened to most come first', () => {
  const artists = topArtists(library, [{ trackId: 'c1', plays: 4 }, { trackId: 'a1', plays: 1 }])
  assert.deepEqual(artists.slice(0, 2).map((artist) => artist.name), ['Cielo Norte', 'Luna Verde'])
})

test('artists sort by name, by listening or by number of songs', () => {
  const artists = topArtists(library, [{ trackId: 'd1', plays: 7 }], Infinity)
  assert.deepEqual(sortArtists(artists, 'name').map((artist) => artist.name), ['Cielo Norte', 'Los Ríos', 'Luna Verde', 'Marea Alta', 'Nadie'])
  assert.equal(sortArtists(artists, 'listened')[0].name, 'Marea Alta')
  assert.equal(sortArtists(artists, 'songs')[0].name, 'Luna Verde')
})

test('initials stand in for an artist without a cover', () => {
  assert.equal(artistInitials('Luna Verde'), 'LV')
  assert.equal(artistInitials('Shakira'), 'S')
  assert.equal(artistInitials('  los   ríos de la plata '), 'LR')
  assert.equal(artistInitials(''), '?')
})

test("an artist's albums are the ones with their songs, newest year first", () => {
  const own = [{ ...song('n1', 'Nuevo', 'Luna Verde'), year: 2022 }, { ...song('v1', 'Viejo', 'Luna Verde'), year: 2001 }, song('o1', 'Otro', 'Los Ríos')] as Track[]
  assert.deepEqual(artistAlbums(nameKey('Luna Verde'), groupAlbums(own)).map((album) => album.name), ['Nuevo', 'Viejo'])
})

test('one artist written with different capitals, accents or spaces is one artist', () => {
  const tracks = [
    song('t1', 'Tattle Tales', '6ix9ine'), song('t2', 'Tattle Tales', '6ix9ine'), song('t3', 'Day69', '6ix9ine'),
    song('t4', 'Day69', '6IX9INE'), song('t5', 'Otra', '6IX9INE '),
    song('b1', 'Lemonade', 'Beyoncé'), song('b2', 'Lemonade', 'beyonce'),
  ]
  const artists = topArtists(tracks, [], Infinity)
  assert.deepEqual(artists.map((artist) => [artist.name, artist.tracks.length]), [['6ix9ine', 5], ['Beyoncé', 2]])
  assert.deepEqual(artistAlbums(artists[0].key, groupAlbums(tracks)).map((album) => album.name).sort(), ['Day69', 'Otra', 'Tattle Tales'])
})

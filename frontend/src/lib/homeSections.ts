// What the home page shows, worked out from the library and the listening history.
import type { Track } from '../../bindings/musica-nativa/internal/library/models'
import { albumKey, compareAlbumOrder, nameKey, sortAlbums, type AlbumGroup } from './albums.ts'

export type TrackPlays = { trackId: string; plays: number }
// key groups the ways the tags write the artist; name is the best-written one.
export type ArtistSummary = { key: string; name: string; tracks: Track[]; plays: number }

const daySeconds = 86400
const rediscoverAfterDays = 30

export function greeting(hour: number): string {
  if (hour >= 6 && hour < 12) return 'Buenos días'
  if (hour >= 12 && hour < 20) return 'Buenas tardes'
  return 'Buenas noches'
}

export function libraryStats(tracks: Track[]) {
  const seconds = tracks.reduce((total, track) => total + (track.durationSeconds || 0), 0)
  return {
    songs: tracks.length,
    albums: new Set(tracks.map(albumKey)).size,
    artists: new Set(tracks.map((track) => track.artist)).size,
    time: seconds < 3600 ? `${Math.round(seconds / 60)} min` : `${Math.round(seconds / 3600)} h`,
  }
}

// seededRandom returns a repeatable sequence of numbers in [0, 1) for a text.
function seededRandom(seed: string): () => number {
  let state = 2166136261
  for (let i = 0; i < seed.length; i++) state = Math.imul(state ^ seed.charCodeAt(i), 16777619)
  return () => {
    state = (state + 0x6d2b79f5) | 0
    let mixed = Math.imul(state ^ (state >>> 15), 1 | state)
    mixed = (mixed + Math.imul(mixed ^ (mixed >>> 7), 61 | mixed)) ^ mixed
    return ((mixed ^ (mixed >>> 14)) >>> 0) / 4294967296
  }
}

function shuffled<T>(items: T[], seed: string): T[] {
  const random = seededRandom(seed)
  const copy = [...items]
  for (let i = copy.length - 1; i > 0; i--) {
    const j = Math.floor(random() * (i + 1))
    ;[copy[i], copy[j]] = [copy[j], copy[i]]
  }
  return copy
}

// dailyMix picks songs at random, the same ones all day (dayKey: YYYY-MM-DD).
export function dailyMix(tracks: Track[], dayKey: string, size = 25): Track[] {
  return shuffled(tracks.filter((track) => track.available), `mezcla-${dayKey}`).slice(0, size)
}

// recentAlbums turns the songs listened to last into their albums, each once.
export function recentAlbums(recentIDs: string[], tracks: Track[], albums: AlbumGroup[], size = 12): AlbumGroup[] {
  const byID = new Map(tracks.map((track) => [track.id, track]))
  const byKey = new Map(albums.map((album) => [album.key, album]))
  const result: AlbumGroup[] = []
  for (const id of recentIDs) {
    const track = byID.get(id)
    const album = track && byKey.get(albumKey(track))
    if (album && !result.includes(album)) result.push(album)
    if (result.length === size) break
  }
  return result
}

export function topTracks(top: TrackPlays[], tracks: Track[], size = 8) {
  const byID = new Map(tracks.map((track) => [track.id, track]))
  return top.flatMap(({ trackId, plays }) => {
    const track = byID.get(trackId)
    return track ? [{ track, plays }] : []
  }).slice(0, size)
}

// rediscover offers albums not heard in the last month (or never), at random;
// a new seed gives a new selection.
export function rediscover(albums: AlbumGroup[], lastPlayed: Record<string, number>, nowSeconds: number, seed: number, size = 8): AlbumGroup[] {
  const forgotten = albums.filter((album) => {
    if (!album.tracks.some((track) => track.available)) return false
    const last = Math.max(0, ...album.tracks.map((track) => lastPlayed[track.id] ?? 0))
    return nowSeconds - last > rediscoverAfterDays * daySeconds
  })
  return shuffled(forgotten, `redescubre-${seed}`).slice(0, size)
}

// topArtists lists artists by how much they were listened to, then by how
// many songs they have. Each artist's songs come album by album, in order.
export function topArtists(tracks: Track[], top: TrackPlays[], size = 10): ArtistSummary[] {
  const plays = new Map(top.map(({ trackId, plays }) => [trackId, plays]))
  const artists = new Map<string, ArtistSummary>()
  const spellings = new Map<string, Map<string, number>>()
  for (const track of tracks) {
    const key = nameKey(track.artist)
    const artist = artists.get(key) ?? { key, name: track.artist.trim(), tracks: [], plays: 0 }
    artist.tracks.push(track)
    artist.plays += plays.get(track.id) ?? 0
    artists.set(key, artist)
    const written = spellings.get(key) ?? new Map<string, number>()
    written.set(track.artist.trim(), (written.get(track.artist.trim()) ?? 0) + 1)
    spellings.set(key, written)
  }
  for (const artist of artists.values()) {
    artist.name = bestSpelling(spellings.get(artist.key)!)
    artist.tracks.sort((a, b) => a.album.localeCompare(b.album) || compareAlbumOrder(a, b))
  }
  return [...artists.values()]
    .sort((a, b) => b.plays - a.plays || b.tracks.length - a.tracks.length || a.name.localeCompare(b.name))
    .slice(0, size)
}

export type ArtistSort = 'name' | 'listened' | 'songs'

export const artistSortNames: Record<ArtistSort, string> = { name: 'Nombre', listened: 'Más escuchados', songs: 'Más canciones' }

const artistNames = new Intl.Collator('es', { numeric: true, sensitivity: 'base' })

export function sortArtists(artists: ArtistSummary[], key: ArtistSort): ArtistSummary[] {
  const byName = (a: ArtistSummary, b: ArtistSummary) => artistNames.compare(a.name, b.name)
  const compare: Record<ArtistSort, (a: ArtistSummary, b: ArtistSummary) => number> = {
    name: byName,
    listened: (a, b) => b.plays - a.plays || b.tracks.length - a.tracks.length || byName(a, b),
    songs: (a, b) => b.tracks.length - a.tracks.length || byName(a, b),
  }
  return [...artists].sort(compare[key])
}

// artistInitials stands in for an artist's picture: the first letters of
// the first two words.
export function artistInitials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  if (!words.length) return '?'
  return words.slice(0, 2).map((word) => word[0].toLocaleUpperCase()).join('')
}

// artistAlbums are the albums with songs by the artist (key from
// ArtistSummary), newest year first.
export function artistAlbums(key: string, albums: AlbumGroup[]): AlbumGroup[] {
  return sortAlbums(albums.filter((album) => nameKey(album.artist) === key || album.tracks.some((track) => nameKey(track.artist) === key)), 'year')
}

// bestSpelling picks how to show a name written in several ways: the most
// used, and on a tie the one with capitals and accents as a person writes it
// ("Beyoncé" over "beyonce", "Shakira" over "SHAKIRA").
function bestSpelling(counts: Map<string, number>): string {
  const score = (name: string) => {
    const mixedCase = name !== name.toLocaleUpperCase() && name !== name.toLocaleLowerCase()
    const accents = name !== name.normalize('NFD')
    return (counts.get(name) ?? 0) * 10 + (mixedCase ? 2 : 0) + (accents ? 1 : 0)
  }
  return [...counts.keys()].reduce((best, name) => (score(name) > score(best) ? name : best))
}

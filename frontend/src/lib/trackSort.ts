// Orders for song lists. "order" keeps the list as given (a playlist's order).
import type { Track } from '../../bindings/musica-nativa/internal/library/models'
import { compareAlbumOrder } from './albums.ts'

export type SortKey = 'order' | 'title' | 'artist' | 'album' | 'duration' | 'added'

export const sortNames: Record<SortKey, string> = {
  order: 'Orden original',
  title: 'Título',
  artist: 'Artista',
  album: 'Álbum',
  duration: 'Duración',
  added: 'Más recientes',
}

const names = new Intl.Collator('es', { numeric: true, sensitivity: 'base' })

const byAlbum = (a: Track, b: Track) =>
  names.compare(a.album, b.album) || names.compare(a.albumArtist || a.artist, b.albumArtist || b.artist) || compareAlbumOrder(a, b)

const comparators: Record<Exclude<SortKey, 'order'>, (a: Track, b: Track) => number> = {
  title: (a, b) => names.compare(a.title, b.title) || names.compare(a.artist, b.artist),
  artist: (a, b) => names.compare(a.artist, b.artist) || byAlbum(a, b),
  album: byAlbum,
  duration: (a, b) => a.durationSeconds - b.durationSeconds || names.compare(a.title, b.title),
  added: (a, b) => b.modifiedUnix - a.modifiedUnix || names.compare(a.title, b.title),
}

// sortTracks returns a sorted copy of tracks; descending reverses the order.
export function sortTracks(tracks: Track[], key: SortKey, descending: boolean): Track[] {
  const sorted = key === 'order' ? [...tracks] : [...tracks].sort(comparators[key])
  return descending ? sorted.reverse() : sorted
}

// formatLength writes a total listening time: "45 min", "1 h 12 min".
export function formatLength(seconds: number): string {
  const minutes = Math.round(seconds / 60)
  const hours = Math.floor(minutes / 60)
  if (!hours) return `${minutes} min`
  return minutes % 60 ? `${hours} h ${minutes % 60} min` : `${hours} h`
}

import type { Track } from '../../bindings/musica-nativa/internal/library/models'

export type AlbumGroup = { key: string; name: string; artist: string; tracks: Track[] }

// A tag that is missing reads as 0: a song without a disc number is on disc 1,
// and a song without a track number goes after the numbered ones.
const disc = (track: Track) => track.discNumber || 1
const position = (track: Track) => track.trackNumber || Number.MAX_SAFE_INTEGER
const fileName = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

// compareAlbumOrder puts an album's songs in the order of the record: by disc,
// then track number, then file name ("9 …" before "10 …").
export function compareAlbumOrder(a: Track, b: Track): number {
  return disc(a) - disc(b) || position(a) - position(b) || fileName.compare(a.path, b.path)
}

// nameKey compares names the way a person reads them: tags often write the
// same artist or album with other capitals, accents or spaces.
export const nameKey = (name: string) => name.normalize('NFD').replace(/[̀-ͯ]/g, '').toLocaleLowerCase().replace(/\s+/g, ' ').trim()

// albumKey identifies a song's album: its name and its album artist.
export const albumKey = (track: Track) => `${nameKey(track.album)}\u0000${nameKey(track.albumArtist || track.artist)}`

// groupAlbums gathers songs by album and album artist, each album in the
// order of the record and the albums sorted by name.
export function groupAlbums(tracks: Track[]): AlbumGroup[] {
  const grouped = new Map<string, Track[]>()
  for (const track of tracks) {
    const key = albumKey(track)
    const albumTracks = grouped.get(key)
    if (albumTracks) albumTracks.push(track)
    else grouped.set(key, [track])
  }
  return [...grouped.entries()]
    .map(([key, albumTracks]) => {
      albumTracks.sort(compareAlbumOrder)
      return { key, name: albumTracks[0].album, artist: albumTracks[0].albumArtist || albumTracks[0].artist, tracks: albumTracks }
    })
    .sort((a, b) => a.name.localeCompare(b.name))
}

export type AlbumSort = 'name' | 'artist' | 'year' | 'added'

export const albumSortNames: Record<AlbumSort, string> = { name: 'Nombre', artist: 'Artista', year: 'Año', added: 'Más recientes' }

const albumNames = new Intl.Collator('es', { numeric: true, sensitivity: 'base' })

// albumYear is the album's release year from its songs' tags, or 0.
export const albumYear = (album: AlbumGroup) => Math.max(0, ...album.tracks.map((track) => track.year || 0))

export const albumSeconds = (album: AlbumGroup) => album.tracks.reduce((total, track) => total + (track.durationSeconds || 0), 0)

const albumAdded = (album: AlbumGroup) => Math.max(0, ...album.tracks.map((track) => track.modifiedUnix || 0))

// sortAlbums returns a sorted copy: by name, by artist, newest year first
// (albums without a year last) or newest files first.
export function sortAlbums(albums: AlbumGroup[], key: AlbumSort): AlbumGroup[] {
  const byName = (a: AlbumGroup, b: AlbumGroup) => albumNames.compare(a.name, b.name) || albumNames.compare(a.artist, b.artist)
  const compare: Record<AlbumSort, (a: AlbumGroup, b: AlbumGroup) => number> = {
    name: byName,
    artist: (a, b) => albumNames.compare(a.artist, b.artist) || albumYear(a) - albumYear(b) || byName(a, b),
    year: (a, b) => (albumYear(b) || -Infinity) - (albumYear(a) || -Infinity) || byName(a, b),
    added: (a, b) => albumAdded(b) - albumAdded(a) || byName(a, b),
  }
  return [...albums].sort(compare[key])
}

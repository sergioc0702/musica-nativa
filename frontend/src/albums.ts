import type { Track } from '../bindings/musica-nativa/internal/library/models'

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

// groupAlbums gathers songs by album and album artist, each album in the
// order of the record and the albums sorted by name.
export function groupAlbums(tracks: Track[]): AlbumGroup[] {
  const grouped = new Map<string, Track[]>()
  for (const track of tracks) {
    const key = `${track.album}\u0000${track.albumArtist || track.artist}`
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

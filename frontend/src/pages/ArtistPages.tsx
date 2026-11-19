import { useMemo, useState, type CSSProperties, type ReactNode } from 'react'
import { ArrowLeft, Disc3, Play, Shuffle, UserRound } from 'lucide-react'
import type { Track } from '../../bindings/musica-nativa/internal/library/models'
import type { AlbumGroup } from '../lib/albums'
import { useCoverColor } from '../lib/coverColor'
import { coverStyle } from '../lib/format'
import { artistInitials, artistSortNames, sortArtists, type ArtistSort, type ArtistSummary } from '../lib/homeSections'
import { formatLength } from '../lib/trackSort'

const artistCover = (artist: ArtistSummary): Track | undefined => artist.tracks.find((track) => track.coverUrl)

// ArtistPicture is the artist's face in the app: the cover of one of their
// albums, or their initials when no song has a cover.
function ArtistPicture({ artist, index, className }: { artist: ArtistSummary; index: number; className: string }) {
  const cover = artistCover(artist)
  return <span className={className} style={coverStyle(cover, index)}>{cover ? null : <span className="artist-initials">{artistInitials(artist.name)}</span>}</span>
}

export function ArtistsPage({ artists, onOpen, onPlay }: { artists: ArtistSummary[]; onOpen: (artist: ArtistSummary) => void; onPlay: (artist: ArtistSummary) => void }) {
  const [sort, setSort] = useState<ArtistSort>('name')
  const sorted = useMemo(() => sortArtists(artists, sort), [artists, sort])
  return <>
    <div className="page-heading">
      <div><h1>Artistas</h1><p>{artists.length} {artists.length === 1 ? 'artista' : 'artistas'}</p></div>
      {artists.length > 1 && <div className="heading-actions"><label className="sort-select"><span>Ordenar por</span>
        <select value={sort} onChange={(event) => setSort(event.target.value as ArtistSort)}>
          {(Object.keys(artistSortNames) as ArtistSort[]).map((key) => <option key={key} value={key}>{artistSortNames[key]}</option>)}
        </select>
      </label></div>}
    </div>
    {sorted.length ? <div className="artist-wall">{sorted.map((artist, index) => <div className="artist-tile" key={artist.name}>
      <button className="artist-tile-open" onClick={() => onOpen(artist)} aria-label={`Abrir ${artist.name}`}>
        <ArtistPicture artist={artist} index={index} className="artist-tile-art" />
        <strong>{artist.name}</strong>
        <small>{artist.tracks.length} {artist.tracks.length === 1 ? 'canción' : 'canciones'}</small>
      </button>
      <button className="artist-tile-play" onClick={() => onPlay(artist)} aria-label={`Reproducir ${artist.name}`}><Play size={16} fill="currentColor" className="play-icon" /></button>
    </div>)}</div> : <div className="empty-state"><div className="empty-icon"><UserRound size={23} /></div><h2>Todavía no hay artistas.</h2><p>Al añadir música, los artistas aparecen aquí.</p></div>}
  </>
}

// ArtistView is one artist: their picture, tinted by its color, their albums
// and then all their songs, album by album.
export function ArtistView({ artist, albums, onBack, onPlay, onShuffle, onOpenAlbum, onPlayAlbum, children }: { artist: ArtistSummary; albums: AlbumGroup[]; onBack: () => void; onPlay: () => void; onShuffle: () => void; onOpenAlbum: (album: AlbumGroup) => void; onPlayAlbum: (album: AlbumGroup) => void; children: ReactNode }) {
  const tint = useCoverColor(artistCover(artist)?.coverUrl)
  const seconds = artist.tracks.reduce((total, track) => total + (track.durationSeconds || 0), 0)
  const songs = artist.tracks.length
  return <div className="artist-view">
    <button className="text-button album-back" onClick={onBack}><ArrowLeft size={15} /> Artistas</button>
    <header className="album-hero artist-hero" style={tint ? { '--tint': tint } as CSSProperties : undefined}>
      <ArtistPicture artist={artist} index={0} className="artist-hero-art" />
      <div className="album-hero-text">
        <h1>{artist.name}</h1>
        <p>{albums.length} {albums.length === 1 ? 'álbum' : 'álbumes'} · {songs} {songs === 1 ? 'canción' : 'canciones'} · {formatLength(seconds)}{artist.plays > 0 ? ` · lo escuchaste ${artist.plays} ${artist.plays === 1 ? 'vez' : 'veces'}` : ''}</p>
        <div className="album-hero-actions">
          <button className="toolbar-play" onClick={onPlay} aria-label={`Reproducir ${artist.name}`} title="Reproducir"><Play size={22} fill="currentColor" className="play-icon" /></button>
          <button className="secondary-button" onClick={onShuffle}><Shuffle size={15} /> Aleatorio</button>
        </div>
      </div>
    </header>
    {albums.length > 0 && <section className="artist-section">
      <h2 className="home-section-title">Álbumes</h2>
      <div className="shelf-row">{albums.map((album, index) => {
        const cover = album.tracks.find((track) => track.coverUrl) ?? album.tracks[0]
        const year = Math.max(0, ...album.tracks.map((track) => track.year || 0))
        return <div className="shelf-tile" key={album.key}>
          <button className="shelf-item" onClick={() => onOpenAlbum(album)} aria-label={`Abrir ${album.name}`}>
            <span className="shelf-art" style={coverStyle(cover, index)}>{cover?.coverUrl ? null : <Disc3 size={30} strokeWidth={1.2} />}</span>
            <strong>{album.name}</strong><small>{year ? year : `${album.tracks.length} ${album.tracks.length === 1 ? 'canción' : 'canciones'}`}</small>
          </button>
          <button className="shelf-play" onClick={() => onPlayAlbum(album)} aria-label={`Reproducir ${album.name}`}><Play size={16} fill="currentColor" /></button>
        </div>
      })}</div>
    </section>}
    <section className="artist-section">
      <h2 className="home-section-title">Canciones</h2>
      {children}
    </section>
  </div>
}

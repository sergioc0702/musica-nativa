import { useMemo, useState, type CSSProperties, type ReactNode } from 'react'
import { ArrowLeft, Disc3, Play, Shuffle } from 'lucide-react'
import { albumSeconds, albumSortNames, albumYear, sortAlbums, type AlbumGroup, type AlbumSort } from '../lib/albums'
import { useCoverColor } from '../lib/coverColor'
import { coverStyle } from '../lib/format'
import { formatLength } from '../lib/trackSort'

const albumCover = (album: AlbumGroup) => album.tracks.find((track) => track.coverUrl) ?? album.tracks[0]

// AlbumsPage is the grid of albums. A click opens the album; the round
// button plays it straight away.
export function AlbumsPage({ albums, onOpen, onPlay }: { albums: AlbumGroup[]; onOpen: (album: AlbumGroup) => void; onPlay: (album: AlbumGroup) => void }) {
  const [sort, setSort] = useState<AlbumSort>('name')
  const sorted = useMemo(() => sortAlbums(albums, sort), [albums, sort])
  return <>
    <div className="page-heading">
      <div><h1>Álbumes</h1><p>{albums.length} {albums.length === 1 ? 'álbum' : 'álbumes'}</p></div>
      {albums.length > 1 && <div className="heading-actions"><label className="sort-select"><span>Ordenar por</span>
        <select value={sort} onChange={(event) => setSort(event.target.value as AlbumSort)}>
          {(Object.keys(albumSortNames) as AlbumSort[]).map((key) => <option key={key} value={key}>{albumSortNames[key]}</option>)}
        </select>
      </label></div>}
    </div>
    {sorted.length ? <div className="album-wall">{sorted.map((album, index) => {
      const cover = albumCover(album)
      const year = albumYear(album)
      return <div className="album-tile" key={album.key}>
        <button className="album-tile-open" onClick={() => onOpen(album)} aria-label={`Abrir ${album.name}, de ${album.artist}`}>
          <span className="album-tile-art" style={coverStyle(cover, index)}>{cover?.coverUrl ? null : <Disc3 size={44} strokeWidth={1.1} />}</span>
          <strong>{album.name}</strong>
          <small>{album.artist}{year ? ` · ${year}` : ''}</small>
        </button>
        <button className="album-tile-play" onClick={() => onPlay(album)} aria-label={`Reproducir ${album.name}`}><Play size={17} fill="currentColor" className="play-icon" /></button>
      </div>
    })}</div> : <div className="empty-state"><div className="empty-icon"><Disc3 size={23} /></div><h2>Aún no hay álbumes.</h2><p>Los álbumes aparecen a partir de las etiquetas de tu música.</p></div>}
  </>
}

// AlbumView is one album: its cover large, tinted by the cover's own color,
// and its songs in the order of the record.
export function AlbumView({ album, onBack, onPlay, onShuffle, children }: { album: AlbumGroup; onBack: () => void; onPlay: () => void; onShuffle: () => void; children: ReactNode }) {
  const cover = albumCover(album)
  const tint = useCoverColor(cover?.coverUrl)
  const year = albumYear(album)
  const songs = album.tracks.length
  const discs = new Set(album.tracks.map((track) => track.discNumber || 1)).size
  return <div className="album-view">
    <button className="text-button album-back" onClick={onBack}><ArrowLeft size={15} /> Álbumes</button>
    <header className="album-hero" style={tint ? { '--tint': tint } as CSSProperties : undefined}>
      <span className="album-hero-art" style={coverStyle(cover, 0)}>{cover?.coverUrl ? null : <Disc3 size={64} strokeWidth={1} />}</span>
      <div className="album-hero-text">
        <h1>{album.name}</h1>
        <p><strong>{album.artist}</strong>{year ? ` · ${year}` : ''} · {songs} {songs === 1 ? 'canción' : 'canciones'}{discs > 1 ? ` en ${discs} discos` : ''} · {formatLength(albumSeconds(album))}</p>
        <div className="album-hero-actions">
          <button className="toolbar-play" onClick={onPlay} aria-label={`Reproducir ${album.name}`} title="Reproducir"><Play size={22} fill="currentColor" className="play-icon" /></button>
          <button className="secondary-button" onClick={onShuffle}><Shuffle size={15} /> Aleatorio</button>
        </div>
      </div>
    </header>
    {children}
  </div>
}

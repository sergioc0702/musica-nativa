import type { CSSProperties, ReactNode } from 'react'
import { Heart, Music2, Play, Shuffle } from 'lucide-react'
import type { Track } from '../bindings/musica-nativa/internal/library/models'
import { useCoverColor } from './coverColor'
import { coverStyle } from './format'
import { formatLength } from './trackSort'

// FavoritesView is the songs marked with a heart: a mosaic of their covers,
// tinted by the first one, and the list below.
export function FavoritesView({ tracks, searching, onPlay, onShuffle, onBrowse, children }: { tracks: Track[]; searching: boolean; onPlay: () => void; onShuffle: () => void; onBrowse: () => void; children: ReactNode }) {
  const seen = new Set<string>()
  const covers = tracks.filter((track) => track.coverUrl && !seen.has(track.coverUrl) && seen.add(track.coverUrl)).slice(0, 4)
  const tint = useCoverColor(covers[0]?.coverUrl)
  const seconds = tracks.reduce((total, track) => total + (track.durationSeconds || 0), 0)

  if (!tracks.length) {
    return <div className="favorites-empty">
      <span className="favorites-empty-icon"><Heart size={30} /></span>
      <h1>{searching ? 'Ninguna favorita coincide' : 'Aún no tienes favoritas'}</h1>
      <p>{searching ? 'Prueba con otra búsqueda.' : 'Toca el corazón de cualquier canción, en una lista o en el reproductor, y aparecerá aquí.'}</p>
      {!searching && <button className="secondary-button" onClick={onBrowse}><Music2 size={15} /> Ir a Canciones</button>}
    </div>
  }

  return <div className="favorites-view">
    <header className="album-hero" style={tint ? { '--tint': tint } as CSSProperties : undefined}>
      <span className={`favorites-mosaic ${covers.length >= 4 ? 'four' : ''}`} style={covers.length ? undefined : coverStyle(undefined, 3)}>
        {(covers.length >= 4 ? covers : covers.slice(0, 1)).map((track) => <span key={track.id} style={coverStyle(track)} />)}
        <span className="favorites-heart"><Heart size={22} fill="currentColor" /></span>
      </span>
      <div className="album-hero-text">
        <h1>Favoritos</h1>
        <p>{tracks.length} {tracks.length === 1 ? 'canción' : 'canciones'} · {formatLength(seconds)}</p>
        <div className="album-hero-actions">
          <button className="toolbar-play" onClick={onPlay} aria-label="Reproducir tus favoritas" title="Reproducir"><Play size={22} fill="currentColor" className="play-icon" /></button>
          <button className="secondary-button" onClick={onShuffle}><Shuffle size={15} /> Aleatorio</button>
        </div>
      </div>
    </header>
    {children}
  </div>
}

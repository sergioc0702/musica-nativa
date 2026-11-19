import { useMemo, useState, type CSSProperties, type ReactNode } from 'react'
import { ArrowUpRight, Disc3, FolderPlus, Heart, Pause, Play, RefreshCw, Shuffle, Sparkles } from 'lucide-react'
import type { ListeningHistory } from '../bindings/musica-nativa/models'
import type { Track } from '../bindings/musica-nativa/internal/library/models'
import type { AlbumGroup } from './albums'
import { useCoverColor } from './coverColor'
import { coverStyle, formatTime } from './format'
import { artistInitials, dailyMix, greeting, libraryStats, recentAlbums, rediscover, topArtists, topTracks } from './homeSections'
import { usePlayback } from './playback'

type HomeProps = {
  tracks: Track[]
  albums: AlbumGroup[]
  history: ListeningHistory
  currentTrack?: Track
  scanning: boolean
  onPlay: (track: Track, list: Track[]) => void
  onPlayAlbum: (album: AlbumGroup) => void
  onOpenAlbum: (album: AlbumGroup) => void
  onOpenArtist: (name: string) => void
  onToggle: () => void
  onResume: () => void
  onShuffleAll: () => void
  onFavorites: () => void
  onBrowse: () => void
  onViewAlbums: () => void
}

export function HomePage({ tracks, albums, history, currentTrack, scanning, onPlay, onPlayAlbum, onOpenAlbum, onOpenArtist, onToggle, onResume, onShuffleAll, onFavorites, onBrowse, onViewAlbums }: HomeProps) {
  const [rediscoverSeed, setRediscoverSeed] = useState(1)
  const now = new Date()
  const dayKey = now.toLocaleDateString('sv') // YYYY-MM-DD, local day
  const stats = useMemo(() => libraryStats(tracks), [tracks])
  const mix = useMemo(() => dailyMix(tracks, dayKey), [tracks, dayKey])
  // A different daily draw, only for the covers of "Toda tu música".
  const everything = useMemo(() => dailyMix(tracks, `todo-${dayKey}`, 60), [tracks, dayKey])
  const favorites = useMemo(() => tracks.filter((track) => track.favorite && track.available), [tracks])
  const recent = useMemo(() => recentAlbums(history.recent ?? [], tracks, albums), [history.recent, tracks, albums])
  const top = useMemo(() => topTracks(history.top ?? [], tracks), [history.top, tracks])
  const artists = useMemo(() => topArtists(tracks, history.top ?? []), [tracks, history.top])
  const lastPlayed = (history.lastPlayed ?? {}) as Record<string, number>
  const forgotten = useMemo(() => rediscover(albums, lastPlayed, Math.floor(Date.now() / 1000), rediscoverSeed), [albums, lastPlayed, rediscoverSeed])
  const newest = useMemo(() => albums
    .map((album) => ({ album, added: Math.max(...album.tracks.map((track) => track.modifiedUnix)) }))
    .sort((a, b) => b.added - a.added).slice(0, 12).map(({ album }) => album), [albums])

  if (!tracks.length) {
    return <section className="empty-library"><div className="empty-icon"><FolderPlus size={22} /></div><div><h2>{scanning ? 'Buscando música en tus carpetas…' : 'Todavía no hay música por aquí.'}</h2><p>Elige una o más carpetas y Música Nativa leerá tus archivos MP3, WAV y FLAC sin moverlos.</p></div><button className="primary-button" onClick={onBrowse} disabled={scanning}><FolderPlus size={16} /> {scanning ? 'Buscando…' : 'Elegir carpetas'}</button></section>
  }

  const resumeTrack = history.resume ? tracks.find((track) => track.id === history.resume?.trackId) : undefined

  return <div className="home">
    <header className="home-greeting">
      <h1>{greeting(now.getHours())}</h1>
      <p>{stats.songs} {stats.songs === 1 ? 'canción' : 'canciones'} · {stats.albums} {stats.albums === 1 ? 'álbum' : 'álbumes'} · {stats.artists} {stats.artists === 1 ? 'artista' : 'artistas'} · {stats.time} de música</p>
    </header>

    <ListenHero current={currentTrack} resume={resumeTrack} resumeMs={history.resume?.positionMs ?? 0} startAlbum={newest[0]}
      onToggle={onToggle} onResume={onResume} onPlayAlbum={onPlayAlbum} />

    <div className="quick-mixes">
      <MixTile title="Toda tu música" detail="En aleatorio" icon={<Shuffle size={15} />} covers={everything} index={0} onPlay={onShuffleAll} />
      {favorites.length > 0 && <MixTile title="Tus favoritas" detail={`${favorites.length} ${favorites.length === 1 ? 'canción' : 'canciones'}`} icon={<Heart size={15} />} covers={favorites} index={1} onPlay={onFavorites} />}
      {mix.length > 1 && <MixTile title="Mezcla del día" detail="Nueva cada día" icon={<Sparkles size={15} />} covers={mix} index={2} onPlay={() => onPlay(mix[0], mix)} />}
    </div>

    {recent.length > 0 && <AlbumRow title="Escuchado recientemente" albums={recent} onPlayAlbum={onPlayAlbum} onOpenAlbum={onOpenAlbum} />}

    <div className={`home-columns ${top.length ? '' : 'single'}`}>
      {top.length > 0 && <section>
        <h2 className="home-section-title">Lo que más escuchas</h2>
        <div className="top-list">{top.map(({ track, plays }, index) => <button className="top-row" key={track.id} onClick={() => onPlay(track, top.map((item) => item.track))}>
          <span className="top-rank">{index + 1}</span>
          <span className="top-cover" style={coverStyle(track, index)}>{track.coverUrl ? null : <Disc3 size={15} />}</span>
          <span className="top-text"><strong>{track.title}</strong><small>{track.artist}</small></span>
          <span className="top-plays">{plays} {plays === 1 ? 'vez' : 'veces'}</span>
        </button>)}</div>
      </section>}
      <section>
        <div className="home-section-head"><h2 className="home-section-title">Añadido recientemente</h2><button className="text-button" onClick={onViewAlbums}>Ver álbumes <ArrowUpRight size={15} /></button></div>
        <div className={top.length ? 'album-mini-grid' : 'shelf-row'}>{newest.slice(0, top.length ? 6 : 12).map((album, index) => <AlbumTile key={album.key} album={album} index={index} onPlayAlbum={onPlayAlbum} onOpenAlbum={onOpenAlbum} />)}</div>
      </section>
    </div>

    {artists.length > 1 && <section>
      <h2 className="home-section-title">Tus artistas</h2>
      <div className="shelf-row artist-row">{artists.map((artist, index) => {
        const cover = artist.tracks.find((track) => track.coverUrl)
        return <div className="shelf-tile artist-bubble-tile" key={artist.name}>
          <button className="artist-bubble" onClick={() => onOpenArtist(artist.key)} aria-label={`Abrir ${artist.name}`}>
            <span className="artist-bubble-art" style={coverStyle(cover, index)}>{cover ? null : <span className="artist-initials">{artistInitials(artist.name)}</span>}</span>
            <strong>{artist.name}</strong><small>{artist.tracks.length} {artist.tracks.length === 1 ? 'canción' : 'canciones'}</small>
          </button>
          <button className="shelf-play" onClick={() => onPlay(artist.tracks[0], artist.tracks)} aria-label={`Reproducir ${artist.name}`}><Play size={14} fill="currentColor" /></button>
        </div>
      })}</div>
    </section>}

    {forgotten.length > 0 && <AlbumRow title="Redescubre" albums={forgotten} onPlayAlbum={onPlayAlbum} onOpenAlbum={onOpenAlbum}
      action={<button className="text-button" onClick={() => setRediscoverSeed((seed) => seed + 1)}><RefreshCw size={14} /> Otros</button>} />}
  </div>
}

function AlbumRow({ title, albums, onPlayAlbum, onOpenAlbum, action }: { title: string; albums: AlbumGroup[]; onPlayAlbum: (album: AlbumGroup) => void; onOpenAlbum: (album: AlbumGroup) => void; action?: ReactNode }) {
  return <section>
    <div className="home-section-head"><h2 className="home-section-title">{title}</h2>{action}</div>
    <div className="shelf-row">{albums.map((album, index) => <AlbumTile key={album.key} album={album} index={index} onPlayAlbum={onPlayAlbum} onOpenAlbum={onOpenAlbum} />)}</div>
  </section>
}

// AlbumTile opens the album; its round button plays it straight away.
function AlbumTile({ album, index, onPlayAlbum, onOpenAlbum }: { album: AlbumGroup; index: number; onPlayAlbum: (album: AlbumGroup) => void; onOpenAlbum: (album: AlbumGroup) => void }) {
  const cover = album.tracks.find((track) => track.coverUrl) ?? album.tracks[0]
  return <div className="shelf-tile">
    <button className="shelf-item" onClick={() => onOpenAlbum(album)} aria-label={`Abrir ${album.name}, de ${album.artist}`}>
      <span className="shelf-art" style={coverStyle(cover, index)}>{cover?.coverUrl ? null : <Disc3 size={30} strokeWidth={1.2} />}</span>
      <strong>{album.name}</strong><small>{album.artist}</small>
    </button>
    <button className="shelf-play" onClick={() => onPlayAlbum(album)} aria-label={`Reproducir ${album.name}`}><Play size={16} fill="currentColor" /></button>
  </div>
}

// ListenHero is the large card at the top: the song playing now, the one
// left halfway last time, or a first album to start with.
function ListenHero({ current, resume, resumeMs, startAlbum, onToggle, onResume, onPlayAlbum }: { current?: Track; resume?: Track; resumeMs: number; startAlbum?: AlbumGroup; onToggle: () => void; onResume: () => void; onPlayAlbum: (album: AlbumGroup) => void }) {
  const isPlaying = usePlayback((playback) => playback.isPlaying)
  const track = current ?? resume ?? startAlbum?.tracks[0]
  if (!track) return null
  const tint = useCoverColor(track.coverUrl)
  const state = current ? (isPlaying ? 'Sonando ahora' : 'En pausa') : resume ? 'Seguir escuchando' : 'Para empezar'
  const title = current || resume ? track.title : startAlbum?.name ?? track.title
  const detail = current || resume ? `${track.artist} · ${track.album}` : `Tu último álbum añadido · ${startAlbum?.artist} · ${startAlbum?.tracks.length} ${startAlbum?.tracks.length === 1 ? 'canción' : 'canciones'}`
  const play = current ? onToggle : resume ? onResume : () => startAlbum && onPlayAlbum(startAlbum)
  return <section className={`listen-hero ${current && isPlaying ? 'playing' : ''}`} aria-label={state} style={tint ? { '--tint': tint } as CSSProperties : undefined}>
    {track.coverUrl && <div className="listen-hero-glow" style={{ backgroundImage: `url("${track.coverUrl}")` }} />}
    <span className="listen-hero-art" style={coverStyle(track, 0)}>{track.coverUrl ? null : <Disc3 size={44} strokeWidth={1.1} />}</span>
    <div className="listen-hero-text">
      <strong>{title}</strong>
      <small>{current && <span className="listening-bars" aria-hidden="true"><i /><i /><i /></span>}{detail}</small>
      {current ? <HeroProgress /> : resume && resumeMs > 0 && track.durationSeconds > 0 && <ProgressLine positionMs={resumeMs} durationMs={track.durationSeconds * 1000} />}
    </div>
    <button className="listen-hero-play" onClick={play} aria-label={current && isPlaying ? 'Pausar' : 'Reproducir'}>{current && isPlaying ? <Pause size={24} fill="currentColor" /> : <Play size={24} fill="currentColor" className="play-icon" />}</button>
  </section>
}

// HeroProgress follows the song that is playing; it re-renders on its own.
function HeroProgress() {
  const positionMs = usePlayback((playback) => playback.positionMs)
  const durationMs = usePlayback((playback) => playback.durationMs)
  return durationMs > 0 ? <ProgressLine positionMs={positionMs} durationMs={durationMs} /> : null
}

function ProgressLine({ positionMs, durationMs }: { positionMs: number; durationMs: number }) {
  return <span className="listen-hero-progress">
    <span className="listen-hero-bar"><span style={{ width: `${Math.min(100, (positionMs / durationMs) * 100)}%` }} /></span>
    <span>{formatTime(positionMs)} / {formatTime(durationMs)}</span>
  </span>
}

// MixTile shows a mix as a mosaic of the covers it will play.
function MixTile({ title, detail, icon, covers, index, onPlay }: { title: string; detail: string; icon: ReactNode; covers: Track[]; index: number; onPlay: () => void }) {
  const seen = new Set<string>()
  const pictures = covers.filter((track) => track.coverUrl && !seen.has(track.coverUrl) && seen.add(track.coverUrl)).slice(0, 4)
  return <button className="quick-mix" onClick={onPlay}>
    <span className={`mix-mosaic ${pictures.length >= 4 ? 'four' : ''}`} style={pictures.length ? undefined : coverStyle(undefined, index)}>
      {(pictures.length >= 4 ? pictures : pictures.slice(0, 1)).map((track) => <span key={track.id} style={coverStyle(track)} />)}
      <span className="mix-badge">{icon}</span>
    </span>
    <span className="mix-text"><strong>{title}</strong><small>{detail}</small></span>
    <span className="mix-play"><Play size={15} fill="currentColor" /></span>
  </button>
}

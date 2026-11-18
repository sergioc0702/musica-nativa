import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type FormEvent, type ReactNode } from 'react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { Events } from '@wailsio/runtime'
import {
  AudioLines, Check, Clock3, Disc3, Ellipsis, Heart, Home, PanelRight,
  Library, ListMusic, Mic2, Music2, Pause, Play, Plus, Search, Settings2,
  Shuffle, SkipBack, SkipForward, SlidersHorizontal, Volume2, X,
  FolderPlus, RefreshCw, Sparkles, ArrowUpRight, Trash2,
} from 'lucide-react'
import { LibraryService } from '../bindings/musica-nativa'
import type { AppState, PlaybackState, ScanProgress, Theme } from '../bindings/musica-nativa/models'
import type { Track } from '../bindings/musica-nativa/internal/library/models'
import '../bindings/events'
import { getPlayback, setPlayback, usePlayback } from './playback'

type AlbumGroup = { key: string; name: string; artist: string; tracks: Track[] }
type View = 'home' | 'tracks' | 'albums' | 'artists' | 'favorites' | 'playlists' | 'queue' | 'settings' | 'playlist'
type UpdatePhase = 'idle' | 'checking' | 'up-to-date' | 'available' | 'downloading' | 'verifying' | 'installing' | 'ready' | 'restarting' | 'error'
type UpdateInfo = { phase: UpdatePhase; currentVersion: string; version?: string; name?: string; notes?: string; written: number; total: number; message?: string }

const defaultUpdate: UpdateInfo = { phase: 'idle', currentVersion: '', written: 0, total: 0 }

const emptyState: AppState = {
  version: '', tracks: [], folders: [], playlists: [], themes: [], outputDevices: [], activeTheme: 'nocturno',
  playback: { path: '', isPlaying: false, positionMs: 0, durationMs: 0, volume: 72, outputMode: 'shared', deviceID: '', outputReady: false, equalizerOn: false, equalizer: [0, 0, 0] },
  scanning: false,
}

const defaultTheme: Theme = {
  id: 'nocturno', name: 'Nocturno', background: '#121214', accent: '#ff7a59',
  surface: '#1d1d21', text: '#f4f4f6', muted: '#a1a1aa', builtIn: true,
}

const navItems: { id: View; label: string; icon: typeof Home }[] = [
  { id: 'home', label: 'Inicio', icon: Home },
  { id: 'tracks', label: 'Canciones', icon: Music2 },
  { id: 'albums', label: 'Álbumes', icon: Disc3 },
  { id: 'artists', label: 'Artistas', icon: Mic2 },
  { id: 'favorites', label: 'Favoritos', icon: Heart },
  { id: 'queue', label: 'Cola', icon: ListMusic },
]

function formatTime(milliseconds: number) {
  const seconds = Math.max(0, Math.floor(milliseconds / 1000))
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
}

function formatDuration(seconds: number) {
  return seconds > 0 ? formatTime(seconds * 1000) : '—:—'
}

function displayError(error: unknown) {
  return error instanceof Error ? error.message : String(error || 'Ocurrió un problema.')
}

function coverStyle(track?: Track, index = 0): CSSProperties {
  if (track?.coverUrl) return { backgroundImage: `url("${track.coverUrl}")` }
  const colors = [
    'linear-gradient(145deg, #b77f5c, #473831 54%, #161817)',
    'linear-gradient(145deg, #8f9c80, #40483e 54%, #161817)',
    'linear-gradient(145deg, #7f8fa2, #39414a 54%, #17191b)',
    'linear-gradient(145deg, #a48d77, #51443a 54%, #171716)',
    'linear-gradient(145deg, #8c7384, #443944 54%, #191719)',
  ]
  return { backgroundImage: colors[index % colors.length] }
}

function App() {
  const reducedMotion = useReducedMotion()
  const [state, setState] = useState<AppState>(emptyState)
  const [view, setView] = useState<View>('home')
  const [query, setQuery] = useState('')
  const searchInput = useRef<HTMLInputElement>(null)
  const [selectedPlaylist, setSelectedPlaylist] = useState('')
  const [queue, setQueue] = useState<Track[]>([])
  const [toast, setToast] = useState('')
  const [busy, setBusy] = useState(false)
  const [update, setUpdate] = useState<UpdateInfo>(defaultUpdate)
  const [updateRunning, setUpdateRunning] = useState(false)
  // Updater errors are shown only once the listener starts an update, not for
  // the quiet check when the app opens.
  const updateRequested = useRef(false)
  const launchCheckDone = useRef(false)
  const [showQueue, setShowQueue] = useState(false)
  const [showNowPlaying, setShowNowPlaying] = useState(() => {
    try { return localStorage.getItem('mn-now-panel') !== 'off' } catch { return true }
  })
  const [playQueue, setPlayQueue] = useState<Track[]>([])
  const [showPlaylistForm, setShowPlaylistForm] = useState(false)
  const [playlistTarget, setPlaylistTarget] = useState<Track | null>(null)
  const [playlistName, setPlaylistName] = useState('')
  const [editor, setEditor] = useState<Theme>({ ...defaultTheme, id: '', name: 'Mi tema' })
  const [imagePreview, setImagePreview] = useState('')

  const refreshState = useCallback(async () => {
    try {
      const next = await LibraryService.AppState()
      setState(next)
      setPlayback(next.playback)
    } catch (error) {
      setToast(displayError(error))
    }
  }, [])

  const refreshQueue = useCallback(async () => {
    try { setQueue(await LibraryService.Queue()) } catch { /* the queue may be empty */ }
  }, [])

  useEffect(() => {
    let mounted = true
    const stopPlayer = Events.On('player:state', (event) => {
      if (!mounted) return
      setPlayback(event.data as PlaybackState)
    })
    const stopScan = Events.On('library:scan-progress', (event) => {
      if (!mounted) return
      const progress = event.data as ScanProgress
      const scanning = progress.phase === 'scanning'
      setState((current) => current.scanning === scanning ? current : { ...current, scanning })
      if (progress.phase === 'done') {
        void refreshState()
        setToast(progress.issues?.[0] || progress.message)
      } else if (progress.phase === 'cancelled') {
        void refreshState()
      }
    })
    const stopDevices = Events.On('audio:devices', () => { void refreshState() })
    void refreshState()
    return () => {
      mounted = false
      stopPlayer()
      stopScan()
      stopDevices()
    }
  }, [refreshState])

  useEffect(() => {
    const stopNoUpdate = Events.On('wails:updater:no-update', () => {
      setUpdate((current) => ({ ...current, phase: 'up-to-date', currentVersion: state.version, version: state.version, written: 0, total: 0, message: '' }))
    })
    const stopAvailable = Events.On('wails:updater:update-available', (event) => {
      const release = event.data as { version: string; name?: string; notes?: string }
      setUpdate((current) => ({ ...current, phase: 'available', version: release.version, name: release.name, notes: release.notes, message: '' }))
    })
    const stopDownload = Events.On('wails:updater:download-started', () => {
      setUpdate((current) => ({ ...current, phase: 'downloading', written: 0, total: 0, message: '' }))
    })
    const stopProgress = Events.On('wails:updater:download-progress', (event) => {
      const progress = event.data as { written: number; total: number }
      setUpdate((current) => ({ ...current, phase: 'downloading', written: progress.written, total: progress.total }))
    })
    const stopVerifying = Events.On('wails:updater:verifying', () => {
      setUpdate((current) => ({ ...current, phase: 'verifying' }))
    })
    const stopInstalling = Events.On('wails:updater:installing', () => {
      setUpdate((current) => ({ ...current, phase: 'installing' }))
    })
    const stopReady = Events.On('wails:updater:update-ready', (event) => {
      const release = event.data as { version: string; name?: string; notes?: string }
      setUpdate((current) => ({ ...current, phase: 'ready', version: release.version, name: release.name || current.name, notes: release.notes || current.notes }))
    })
    const stopError = Events.On('wails:updater:error', (event) => {
      if (!updateRequested.current) return
      const error = event.data as { message?: string }
      setUpdate((current) => ({ ...current, phase: 'error', message: error.message || 'No se pudo completar la actualización.' }))
    })
    return () => {
      stopNoUpdate()
      stopAvailable()
      stopDownload()
      stopProgress()
      stopVerifying()
      stopInstalling()
      stopReady()
      stopError()
    }
  }, [state.version])

  useEffect(() => {
    // Look for a newer version once, quietly, when the app opens.
    if (!state.version || launchCheckDone.current) return
    launchCheckDone.current = true
    LibraryService.CheckForUpdates().then((result) => {
      if (!result.available) return
      setUpdate((current) => current.phase === 'idle' || current.phase === 'up-to-date' || current.phase === 'available'
        ? { ...current, phase: 'available', version: result.version, name: result.name, notes: result.notes }
        : current)
    }).catch(() => { /* offline or no release yet: the button still works */ })
  }, [state.version])

  useEffect(() => {
    const handleShortcut = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        searchInput.current?.focus()
      }
    }
    window.addEventListener('keydown', handleShortcut)
    return () => window.removeEventListener('keydown', handleShortcut)
  }, [])

  const activeTheme = useMemo(
    () => state.themes.find((theme) => theme.id === state.activeTheme) || defaultTheme,
    [state.activeTheme, state.themes],
  )
  const playbackPath = usePlayback((playback) => playback.path)
  useEffect(() => {
    try { localStorage.setItem('mn-now-panel', showNowPlaying ? 'on' : 'off') } catch { /* private window */ }
  }, [showNowPlaying])
  useEffect(() => {
    if (!playbackPath) return
    LibraryService.Queue().then(setPlayQueue).catch(() => setPlayQueue([]))
  }, [playbackPath, queue])
  const isPlaying = usePlayback((playback) => playback.isPlaying)
  const currentTrack = playbackPath ? state.tracks.find((track) => track.path === playbackPath) ?? queue.find((track) => track.path === playbackPath) : undefined
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const searchedTracks = useMemo(() => state.tracks.filter((track) => {
    if (!normalizedQuery) return true
    return `${track.title} ${track.artist} ${track.album}`.toLocaleLowerCase().includes(normalizedQuery)
  }), [normalizedQuery, state.tracks])
  const favoriteTracks = useMemo(() => searchedTracks.filter((track) => track.favorite), [searchedTracks])
  const albums = useMemo(() => {
    const grouped = new Map<string, Track[]>()
    for (const track of state.tracks) {
      const key = `${track.album}\u0000${track.albumArtist || track.artist}`
      const albumTracks = grouped.get(key)
      if (albumTracks) albumTracks.push(track)
      else grouped.set(key, [track])
    }
    return [...grouped.entries()].map(([key, tracks]) => ({ key, name: tracks[0].album, artist: tracks[0].albumArtist || tracks[0].artist, tracks }))
      .sort((a, b) => a.name.localeCompare(b.name))
  }, [state.tracks])
  const tracksByArtist = useMemo(() => {
    const grouped = new Map<string, Track[]>()
    for (const track of state.tracks) {
      const artistTracks = grouped.get(track.artist)
      if (artistTracks) artistTracks.push(track)
      else grouped.set(track.artist, [track])
    }
    return grouped
  }, [state.tracks])
  const artists = useMemo(() => [...tracksByArtist.keys()].sort((a, b) => a.localeCompare(b)), [tracksByArtist])
  useEffect(() => {
    if (!toast) return
    const timeout = window.setTimeout(() => setToast(''), 4200)
    return () => window.clearTimeout(timeout)
  }, [toast])

  const notify = (message: string) => setToast(message)

  // One click: the backend checks, downloads, verifies and restarts the app.
  const updateNow = async () => {
    if (updateRunning) return
    updateRequested.current = true
    setUpdateRunning(true)
    setUpdate((current) => ({ ...current, phase: 'checking', currentVersion: state.version, written: 0, total: 0, message: '' }))
    try {
      const result = await LibraryService.UpdateNow()
      if (!result.available) {
        setUpdate({ phase: 'up-to-date', currentVersion: result.currentVersion, version: result.version, written: 0, total: 0 })
        setUpdateRunning(false)
        return
      }
      // The app is closing to open again on the new version.
      setUpdate((current) => ({ ...current, phase: 'restarting', version: result.version || current.version, name: result.name || current.name }))
    } catch (error) {
      setUpdate((current) => ({ ...current, phase: 'error', message: displayError(error) }))
      setUpdateRunning(false)
    }
  }

  const loadPlaylist = async (playlistID: string) => {
    setSelectedPlaylist(playlistID)
    setView('playlist')
    try {
      const tracks = await LibraryService.GetPlaylistTracks(playlistID)
      setQueue(tracks)
    } catch (error) { notify(displayError(error)) }
  }

  const playTrack = async (track: Track, list = searchedTracks) => {
    try {
      const playable = list.filter((item) => item.available)
      const index = playable.findIndex((item) => item.id === track.id)
      await LibraryService.SetQueue(playable.map((item) => item.id), Math.max(0, index))
      await LibraryService.PlayTrack(track.id)
      await refreshQueue()
    } catch (error) { notify(displayError(error)) }
  }

  const togglePlay = async () => {
    if (!getPlayback().path) {
      const first = searchedTracks.find((track) => track.available)
      if (first) await playTrack(first)
      return
    }
    try {
      setPlayback(await LibraryService.TogglePlayback())
    }
    catch (error) { notify(displayError(error)) }
  }

  const playNext = async () => {
    try { await LibraryService.NextTrack() } catch (error) { notify(displayError(error)) }
  }

  const playPrevious = async () => {
    try { await LibraryService.PreviousTrack() } catch (error) { notify(displayError(error)) }
  }

  const toggleFavorite = async (track: Track) => {
    try {
      await LibraryService.ToggleFavorite(track.id)
      await refreshState()
    } catch (error) { notify(displayError(error)) }
  }

  const startScan = async () => {
    if (state.folders.length === 0) {
      await chooseFolders()
      return
    }
    try {
      setBusy(true)
      const started = await LibraryService.RefreshLibrary()
      if (!started) notify('El escaneo ya está en curso.')
    } catch (error) { notify(displayError(error)) }
    finally { setBusy(false) }
  }

  const chooseFolders = async () => {
    try {
      setBusy(true)
      const folders = await LibraryService.ChooseMusicFolders()
      if (folders.length) await refreshState()
    } catch (error) { notify(displayError(error)) }
    finally { setBusy(false) }
  }

  const createPlaylist = async (event: FormEvent) => {
    event.preventDefault()
    if (!playlistName.trim()) return
    try {
      const created = await LibraryService.CreatePlaylist(playlistName)
      const addedTrack = playlistTarget
      if (addedTrack) {
        await LibraryService.AddTrackToPlaylist(created.id, addedTrack.id)
        setPlaylistTarget(null)
      }
      setPlaylistName('')
      setShowPlaylistForm(false)
      await refreshState()
      notify(addedTrack ? 'Playlist creada y canción añadida.' : 'Playlist creada.')
    } catch (error) { notify(displayError(error)) }
  }

  const addToQueue = async (track: Track) => {
    try {
      await LibraryService.AddToQueue(track.id)
      await refreshQueue()
      notify(`“${track.title}” se añadió a la cola.`)
    } catch (error) { notify(displayError(error)) }
  }

  const addToPlaylist = (track: Track) => {
    setPlaylistTarget(track)
    if (!state.playlists.length) {
      setShowPlaylistForm(true)
      notify('Crea una playlist para guardar esta canción.')
      return
    }
  }

  const choosePlaylist = async (playlistID: string) => {
    const track = playlistTarget
    if (!track) return
    try {
      await LibraryService.AddTrackToPlaylist(playlistID, track.id)
      const playlist = state.playlists.find((item) => item.id === playlistID)
      await refreshState()
      setPlaylistTarget(null)
      notify(playlist ? `Añadida a “${playlist.name}”.` : 'Canción añadida a la playlist.')
    } catch (error) { notify(displayError(error)) }
  }

  const setAudioOptions = async (mode: string, deviceID: string, enabled: boolean, bands: number[]) => {
    try {
      const next = await LibraryService.SetAudioOptions(mode, deviceID, enabled, bands)
      setPlayback(next)
      if (next.error) notify(next.error)
    } catch (error) { notify(displayError(error)) }
  }

  const selectTheme = async (themeID: string) => {
    try {
      await LibraryService.ApplyTheme(themeID)
      setState((current) => ({ ...current, activeTheme: themeID }))
      notify('Tema aplicado a toda la aplicación.')
    } catch (error) { notify(displayError(error)) }
  }

  const saveTheme = async () => {
    const nextTheme = { ...editor, backgroundImage: imagePreview || editor.backgroundImage || '' }
    try {
      const result = await LibraryService.SaveTheme(nextTheme)
      setState((current) => ({ ...current, themes: result.themes }))
      setEditor(result.theme)
      setImagePreview('')
      notify('Tema guardado. Elige “Aplicar” para usarlo.')
    } catch (error) { notify(displayError(error)) }
  }

  const readBackground = (file?: File) => {
    if (!file) return
    if (file.size > 8 * 1024 * 1024) {
      notify('La imagen debe pesar menos de 8 MB.')
      return
    }
    const reader = new FileReader()
    reader.onload = () => setImagePreview(String(reader.result || ''))
    reader.readAsDataURL(file)
  }

  const themeVariables = {
    '--app-bg': activeTheme.background,
    '--accent': activeTheme.accent,
    '--surface': activeTheme.surface,
    '--text': activeTheme.text,
    '--muted': activeTheme.muted,
  } as CSSProperties

  const selectedPlaylistItem = state.playlists.find((playlist) => playlist.id === selectedPlaylist)
  const visibleTracks = view === 'favorites' ? favoriteTracks : searchedTracks
  const title = view === 'home' ? 'Inicio' : view === 'tracks' ? 'Canciones' : view === 'albums' ? 'Álbumes' : view === 'artists' ? 'Artistas' : view === 'favorites' ? 'Tus favoritos' : view === 'queue' ? 'Cola de reproducción' : view === 'settings' ? 'Ajustes' : view === 'playlist' ? selectedPlaylistItem?.name || 'Playlist' : 'Tus playlists'

  return (
    <div className={`app-shell ${showNowPlaying ? 'with-panel' : ''}`} style={themeVariables}>
      {activeTheme.backgroundImage && <div className="theme-backdrop" style={{ backgroundImage: `linear-gradient(90deg, var(--app-bg) 0%, color-mix(in srgb, var(--app-bg) 86%, transparent) 58%, color-mix(in srgb, var(--app-bg) 66%, transparent) 100%), linear-gradient(180deg, color-mix(in srgb, var(--app-bg) 72%, transparent), var(--app-bg)), url("${activeTheme.backgroundImage}")` }} />}
      <aside className="sidebar">
        <button className="brand" onClick={() => setView('home')} aria-label="Música Nativa, inicio">
          <span className="brand-mark"><span className="brand-ring" /><Disc3 size={19} strokeWidth={1.65} /></span>
          <span className="brand-wordmark">música<span>nativa</span></span>
        </button>

        <nav className="primary-nav" aria-label="Biblioteca">
          {navItems.map(({ id, label, icon: Icon }) => <button key={id} className={`nav-item ${view === id ? 'active' : ''}`} aria-label={label} aria-current={view === id ? 'page' : undefined} title={label} onClick={() => { setView(id); if (id === 'queue') void refreshQueue() }}><Icon size={18} strokeWidth={1.8} /><span>{label}</span>{id === 'tracks' && state.tracks.length > 0 && <small>{state.tracks.length}</small>}</button>)}
        </nav>

        <div className="sidebar-rule" />
        <div className="sidebar-section-heading"><span>Playlists</span><button className="icon-button tiny" onClick={() => setShowPlaylistForm(true)} aria-label="Crear playlist"><Plus size={16} /></button></div>
        <nav className="playlist-nav" aria-label="Playlists">
          {state.playlists.slice(0, 6).map((playlist) => <button key={playlist.id} className={`nav-item playlist-link ${view === 'playlist' && selectedPlaylist === playlist.id ? 'active' : ''}`} aria-label={`Playlist ${playlist.name}`} aria-current={view === 'playlist' && selectedPlaylist === playlist.id ? 'page' : undefined} title={playlist.name} onClick={() => void loadPlaylist(playlist.id)}><ListMusic size={17} /><span>{playlist.name}</span></button>)}
          {!state.playlists.length && <span className="sidebar-empty">Tus playlists aparecerán aquí.</span>}
        </nav>

        <div className="sidebar-bottom">
          <button className={`nav-item settings-link ${view === 'settings' ? 'active' : ''}`} aria-label={update.phase === 'available' ? 'Ajustes, hay una versión nueva' : 'Ajustes'} aria-current={view === 'settings' ? 'page' : undefined} title={update.phase === 'available' ? 'Ajustes · versión nueva disponible' : 'Ajustes'} onClick={() => setView('settings')}><Settings2 size={18} /><span>Ajustes</span>{update.phase === 'available' && <i className="update-dot" aria-hidden="true" />}</button>
        </div>
      </aside>

      <main className="main-column">
        <header className="topbar">
          <label className="search-box"><Search size={17} /><input ref={searchInput} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Busca canciones, artistas, álbumes…" aria-label="Buscar en tu biblioteca" /><kbd>Ctrl K</kbd></label>
          <button className="topbar-library-button" disabled={busy} onClick={() => void chooseFolders()}><FolderPlus size={16} /><span>Añadir música</span></button>
        </header>

        <div className="scroll-area">
          <AnimatePresence mode="wait" initial={!reducedMotion}>
            <motion.section key={`${view}-${selectedPlaylist}`} className="page-content" initial={reducedMotion ? false : { opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} exit={reducedMotion ? undefined : { opacity: 0, y: -5 }} transition={{ duration: reducedMotion ? 0 : 0.2, ease: 'easeOut' }}>
              {view === 'home' && <HomePage tracks={state.tracks} albums={albums} currentTrack={currentTrack} isPlaying={isPlaying} scanning={state.scanning} onPlay={(track) => void playTrack(track, state.tracks)} onPlayAlbum={(album) => void playTrack(album.tracks[0], album.tracks)} onToggle={() => void togglePlay()} onBrowse={() => void chooseFolders()} onViewTracks={() => setView('tracks')} onViewAlbums={() => setView('albums')} />}
              {view === 'tracks' && <TrackPage title={title} tracks={visibleTracks} currentPath={playbackPath} isPlaying={isPlaying} onPlay={(track) => void playTrack(track, visibleTracks)} onFavorite={(track) => void toggleFavorite(track)} onQueue={(track) => void addToQueue(track)} onPlaylist={(track) => void addToPlaylist(track)} onRefresh={() => void startScan()} onChooseFolders={() => void chooseFolders()} scanning={state.scanning} />}
              {view === 'favorites' && <TrackPage title={title} tracks={visibleTracks} currentPath={playbackPath} isPlaying={isPlaying} onPlay={(track) => void playTrack(track, visibleTracks)} onFavorite={(track) => void toggleFavorite(track)} onQueue={(track) => void addToQueue(track)} onPlaylist={(track) => void addToPlaylist(track)} onRefresh={() => void startScan()} onChooseFolders={() => void chooseFolders()} scanning={state.scanning} />}
              {view === 'albums' && <AlbumPage albums={albums.filter((album) => !normalizedQuery || `${album.name} ${album.artist}`.toLocaleLowerCase().includes(normalizedQuery))} onPlay={(album) => { void playTrack(album.tracks[0], album.tracks) }} />}
              {view === 'artists' && <ArtistPage artists={artists.filter((artist) => !normalizedQuery || artist.toLocaleLowerCase().includes(normalizedQuery))} tracksByArtist={tracksByArtist} onPlay={(artistTracks) => { if (artistTracks[0]) void playTrack(artistTracks[0], artistTracks) }} />}
              {view === 'playlists' && <PlaylistPage playlists={state.playlists} onOpen={(id) => void loadPlaylist(id)} onCreate={() => setShowPlaylistForm(true)} />}
              {view === 'playlist' && <PlaylistDetail playlist={selectedPlaylistItem} tracks={queue} currentPath={playbackPath} isPlaying={isPlaying} onPlay={(track) => void playTrack(track, queue)} onFavorite={(track) => void toggleFavorite(track)} onQueue={(track) => void addToQueue(track)} onPlaylist={(track) => void addToPlaylist(track)} onRefresh={refreshQueue} />}
              {view === 'queue' && <QueuePage tracks={queue} currentPath={playbackPath} onPlay={(track) => void playTrack(track, queue)} />}
              {view === 'settings' && <SettingsPage state={state} activeTheme={activeTheme} editor={editor} imagePreview={imagePreview} update={update} updateRunning={updateRunning} onUpdate={() => void updateNow()} onTheme={(theme) => void selectTheme(theme)} onEdit={setEditor} onSave={() => void saveTheme()} onImage={readBackground} onAudio={setAudioOptions} onChooseFolders={() => void chooseFolders()} onRemoveFolder={async (path) => { try { await LibraryService.RemoveMusicFolder(path); await refreshState() } catch (error) { notify(displayError(error)) } }} onRefresh={() => void startScan()} />}
            </motion.section>
          </AnimatePresence>
        </div>

      </main>

      {showNowPlaying && <NowPlayingPanel track={currentTrack} queue={playQueue} onPlay={(track) => void playTrack(track, playQueue)} onFavorite={currentTrack ? () => void toggleFavorite(currentTrack) : undefined} onClose={() => setShowNowPlaying(false)} />}
      <PlayerBar track={currentTrack} onToggle={() => void togglePlay()} onNext={() => void playNext()} onPrevious={() => void playPrevious()} onSeek={(value) => LibraryService.SeekTo(value).then(setPlayback)} onVolume={(value) => { void LibraryService.SetVolume(value).then(setPlayback) }} onFavorite={currentTrack ? () => void toggleFavorite(currentTrack) : undefined} onQueue={() => { setShowQueue((current) => !current); void refreshQueue() }} onSettings={() => setView('settings')} panelOpen={showNowPlaying} onTogglePanel={() => setShowNowPlaying((open) => !open)} />

      <AnimatePresence>
        {showQueue && <motion.aside className="queue-drawer" initial={reducedMotion ? false : { x: 22, opacity: 0 }} animate={{ x: 0, opacity: 1 }} exit={{ x: 22, opacity: 0 }} transition={{ duration: reducedMotion ? 0 : 0.18 }}><QueuePage tracks={queue} currentPath={playbackPath} onPlay={(track) => void playTrack(track, queue)} onClose={() => setShowQueue(false)} /></motion.aside>}
      </AnimatePresence>

      <ScanStatus onViewLibrary={() => setView('tracks')} />
      <AnimatePresence>{toast && <motion.div className="toast" role="status" initial={reducedMotion ? false : { opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: 5 }} transition={{ duration: reducedMotion ? 0 : 0.16 }}><span className="toast-dot" />{toast}<button onClick={() => setToast('')} aria-label="Cerrar aviso"><X size={15} /></button></motion.div>}</AnimatePresence>

      <AnimatePresence>
        {showPlaylistForm && <motion.div className="modal-backdrop" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} onMouseDown={(event) => { if (event.target === event.currentTarget) { setShowPlaylistForm(false); setPlaylistTarget(null) } }}><motion.form className="dialog-card" onSubmit={createPlaylist} initial={reducedMotion ? false : { opacity: 0, y: 10, scale: 0.985 }} animate={{ opacity: 1, y: 0, scale: 1 }} exit={{ opacity: 0, y: 6 }}><div className="dialog-title-row"><span className="dialog-icon"><ListMusic size={19} /></span><button type="button" className="icon-button" onClick={() => { setShowPlaylistForm(false); setPlaylistTarget(null) }} aria-label="Cerrar"><X size={17} /></button></div><h2>Crea un espacio propio.</h2><p>Una playlist para el momento, el viaje o el estado de ánimo.</p><input autoFocus value={playlistName} onChange={(event) => setPlaylistName(event.target.value)} placeholder="Nombre de la playlist" maxLength={60} /><button className="primary-button full-button" type="submit" disabled={!playlistName.trim()}>Crear playlist <ArrowUpRight size={16} /></button></motion.form></motion.div>}
      </AnimatePresence>
      <AnimatePresence>
        {playlistTarget && state.playlists.length > 0 && !showPlaylistForm && <motion.div className="modal-backdrop" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} onMouseDown={(event) => { if (event.target === event.currentTarget) setPlaylistTarget(null) }}><motion.div className="dialog-card playlist-picker" initial={reducedMotion ? false : { opacity: 0, y: 10, scale: 0.985 }} animate={{ opacity: 1, y: 0, scale: 1 }} exit={{ opacity: 0, y: 6 }}><div className="dialog-title-row"><span className="dialog-icon"><ListMusic size={19} /></span><button className="icon-button" onClick={() => setPlaylistTarget(null)} aria-label="Cerrar"><X size={17} /></button></div><h2>Añade a una playlist.</h2><p>{playlistTarget.title}</p><div className="playlist-picker-list">{state.playlists.map((playlist) => <button key={playlist.id} onClick={() => void choosePlaylist(playlist.id)}><ListMusic size={15} /><span>{playlist.name}</span><ArrowUpRight size={14} /></button>)}</div><button className="quiet-button full-button" onClick={() => { setPlaylistName(''); setShowPlaylistForm(true) }}><Plus size={15} /> Crear una nueva playlist</button></motion.div></motion.div>}
      </AnimatePresence>
    </div>
  )
}

function PageHeading({ title, subtitle, action }: { title: string; subtitle?: string; action?: ReactNode }) {
  return <div className="page-heading"><div><h1>{title}</h1>{subtitle && <p>{subtitle}</p>}</div>{action && <div className="heading-actions">{action}</div>}</div>
}

function HomePage({ tracks, albums, currentTrack, isPlaying, scanning, onPlay, onPlayAlbum, onToggle, onBrowse, onViewTracks, onViewAlbums }: { tracks: Track[]; albums: AlbumGroup[]; currentTrack?: Track; isPlaying: boolean; scanning: boolean; onPlay: (track: Track) => void; onPlayAlbum: (album: AlbumGroup) => void; onToggle: () => void; onBrowse: () => void; onViewTracks: () => void; onViewAlbums: () => void }) {
  if (!tracks.length) {
    return <><PageHeading title="Inicio" /><section className="empty-library"><div className="empty-icon"><FolderPlus size={22} /></div><div><h2>{scanning ? 'Buscando música en tus carpetas…' : 'Todavía no hay música por aquí.'}</h2><p>Elige una o más carpetas y Música Nativa leerá tus archivos MP3, WAV y FLAC sin moverlos.</p></div><button className="primary-button" onClick={onBrowse} disabled={scanning}><FolderPlus size={16} /> {scanning ? 'Buscando…' : 'Elegir carpetas'}</button></section></>
  }
  const newestTracks = tracks.filter((track) => track.available).sort((a, b) => b.modifiedUnix - a.modifiedUnix).slice(0, 6)
  const newestAlbums = albums
    .map((album) => ({ album, added: Math.max(...album.tracks.map((track) => track.modifiedUnix)) }))
    .sort((a, b) => b.added - a.added)
    .slice(0, 10)
    .map(({ album }) => album)
  const startAlbum = newestAlbums[0]
  return <>
    <section className="continue-section">
      <h2>{currentTrack ? (isPlaying ? 'Sonando ahora' : 'Seguir escuchando') : 'Para empezar'}</h2>
      {currentTrack ? <div className="continue-card">
        <span className="continue-art" style={coverStyle(currentTrack, 0)}>{currentTrack.coverUrl ? null : <Disc3 size={30} strokeWidth={1.3} />}</span>
        <span className="continue-text"><strong>{currentTrack.title}</strong><small>{currentTrack.artist} · {currentTrack.album}</small></span>
        <button className="continue-play" onClick={onToggle} aria-label={isPlaying ? 'Pausar' : 'Reanudar'}>{isPlaying ? <Pause size={20} fill="currentColor" /> : <Play size={20} fill="currentColor" />}</button>
      </div> : startAlbum && <div className="continue-card">
        <span className="continue-art" style={coverStyle(startAlbum.tracks[0], 0)}>{startAlbum.tracks[0]?.coverUrl ? null : <Disc3 size={30} strokeWidth={1.3} />}</span>
        <span className="continue-text"><strong>{startAlbum.name}</strong><small>{startAlbum.artist} · {startAlbum.tracks.length} {startAlbum.tracks.length === 1 ? 'canción' : 'canciones'}</small></span>
        <button className="continue-play" onClick={() => onPlayAlbum(startAlbum)} aria-label={`Reproducir ${startAlbum.name}`}><Play size={20} fill="currentColor" /></button>
      </div>}
    </section>

    <div className="section-heading"><h2>Añadido recientemente</h2><button className="text-button" onClick={onViewAlbums}>Ver álbumes <ArrowUpRight size={15} /></button></div>
    <div className="album-shelf">{newestAlbums.map((album, index) => <button className="shelf-item" key={album.key} onClick={() => onPlayAlbum(album)} aria-label={`Reproducir ${album.name}, de ${album.artist}`}><span className="shelf-art" style={coverStyle(album.tracks[0], index)}>{album.tracks[0]?.coverUrl ? null : <Disc3 size={34} strokeWidth={1.2} />}<span className="shelf-play"><Play size={16} fill="currentColor" /></span></span><strong>{album.name}</strong><small>{album.artist}</small></button>)}</div>

    <div className="section-heading"><h2>Canciones nuevas</h2><button className="text-button" onClick={onViewTracks}>Ver canciones <ArrowUpRight size={15} /></button></div>
    <div className="recent-list">{newestTracks.map((track, index) => <button className="recent-track" key={track.id} onClick={() => onPlay(track)}><span className="recent-cover" style={coverStyle(track, index)}>{track.coverUrl ? null : <Music2 size={17} />}</span><span className="recent-info"><strong>{track.title}</strong><small>{track.artist} · {track.album}</small></span><span className="recent-duration">{formatDuration(track.durationSeconds)}</span><span className="recent-play"><Play size={14} fill="currentColor" /></span></button>)}</div>
  </>
}

// NowPlayingPanel shows the current song large, with what plays next.
function NowPlayingPanel({ track, queue, onPlay, onFavorite, onClose }: { track?: Track; queue: Track[]; onPlay: (track: Track) => void; onFavorite?: () => void; onClose: () => void }) {
  const position = track ? queue.findIndex((item) => item.path === track.path) : -1
  const upNext = position >= 0 ? queue.slice(position + 1, position + 6) : []
  return <aside className="now-panel" aria-label="Reproduciendo">
    {track?.coverUrl && <div className="now-panel-glow" style={{ backgroundImage: `url("${track.coverUrl}")` }} />}
    <div className="now-panel-head"><span>Reproduciendo</span><button className="icon-button" onClick={onClose} aria-label="Ocultar el panel Reproduciendo"><X size={16} /></button></div>
    {track ? <>
      <div className="now-panel-art" style={coverStyle(track, 0)}>{track.coverUrl ? null : <Disc3 size={64} strokeWidth={1} />}</div>
      <div className="now-panel-meta">
        <div><strong>{track.title}</strong><small>{track.artist}</small><small>{track.album}</small></div>
        {onFavorite && <button className={`player-heart ${track.favorite ? 'liked' : ''}`} onClick={onFavorite} aria-label={track.favorite ? 'Quitar de favoritos' : 'Añadir a favoritos'}><Heart size={18} fill={track.favorite ? 'currentColor' : 'none'} /></button>}
      </div>
      <div className="now-panel-label">A continuación</div>
      {upNext.length ? <div className="up-next">{upNext.map((item, index) => <button className="up-next-row" key={`${item.id}-${index}`} onClick={() => onPlay(item)}><span className="up-next-art" style={coverStyle(item, index)}>{item.coverUrl ? null : <Music2 size={13} />}</span><span><strong>{item.title}</strong><small>{item.artist}</small></span><span className="up-next-duration">{formatDuration(item.durationSeconds)}</span></button>)}</div> : <p className="now-panel-empty">No hay más canciones en la cola.</p>}
    </> : <div className="now-panel-idle"><Disc3 size={44} strokeWidth={1.1} /><p>Elige una canción y aquí verás su portada y lo que sigue.</p></div>}
  </aside>
}

function TrackPage({ title, tracks, currentPath, isPlaying, onPlay, onFavorite, onQueue, onPlaylist, onRefresh, onChooseFolders, scanning, showHeading = true }: { title: string; tracks: Track[]; currentPath: string; isPlaying: boolean; onPlay: (track: Track) => void; onFavorite: (track: Track) => void; onQueue: (track: Track) => void; onPlaylist: (track: Track) => void; onRefresh: () => void; onChooseFolders: () => void; scanning: boolean; showHeading?: boolean }) {
  return <>
    {showHeading && <PageHeading title={title} subtitle={`${tracks.length} ${tracks.length === 1 ? 'canción' : 'canciones'} en tu biblioteca local.`} action={<><button className="secondary-button compact-button" onClick={onRefresh} disabled={scanning}><RefreshCw className={scanning ? 'spinning' : ''} size={15} />{scanning ? 'Actualizando' : 'Actualizar'}</button><button className="primary-button compact-button" onClick={onChooseFolders}><FolderPlus size={15} /> Añadir carpeta</button></>} />}
    {tracks.length ? <div className="track-table-wrap" role="region" aria-label="Canciones; desplázate horizontalmente para ver todas las columnas" tabIndex={0}><table className="track-table"><thead><tr><th className="number-col">#</th><th>Título</th><th>Álbum</th><th>Formato</th><th><Clock3 size={14} /></th><th /></tr></thead><tbody>{tracks.map((track, index) => <tr key={track.id} className={`${currentPath === track.path ? 'current' : ''} ${track.available ? '' : 'missing'}`} onDoubleClick={() => track.available && onPlay(track)}><td className="row-number"><button className="row-play" onClick={() => track.available && onPlay(track)} aria-label={`Reproducir ${track.title}`}>{currentPath === track.path && isPlaying ? <AudioLines size={16} /> : <><span>{String(index + 1).padStart(2, '0')}</span><Play size={14} fill="currentColor" /></>}</button></td><td><button className="track-cell" onClick={() => track.available && onPlay(track)}><span className="table-cover" style={coverStyle(track, index)}>{track.coverUrl ? null : <Music2 size={14} />}</span><span className="table-title"><strong>{track.title}</strong><small>{track.artist}{!track.available && <em> · Archivo no disponible</em>}</small></span></button></td><td className="album-cell">{track.album}</td><td><span className="format-tag">{track.format}</span></td><td className="duration-cell">{formatDuration(track.durationSeconds)}</td><td><div className="row-actions"><button className={`row-icon ${track.favorite ? 'liked' : ''}`} onClick={() => onFavorite(track)} aria-label={track.favorite ? 'Quitar de favoritos' : 'Añadir a favoritos'}><Heart size={15} fill={track.favorite ? 'currentColor' : 'none'} /></button><button className="row-icon" onClick={() => onQueue(track)} aria-label="Añadir a la cola"><Plus size={16} /></button><TrackMenu onPlaylist={() => onPlaylist(track)} onQueue={() => onQueue(track)} /></div></td></tr>)}</tbody></table></div> : <div className="empty-state"><div className="empty-icon"><Music2 size={23} /></div><h2>No encontramos canciones.</h2><p>Prueba con otra búsqueda o conecta una carpeta con música.</p><button className="secondary-button" onClick={onChooseFolders}><FolderPlus size={15} /> Añadir carpeta</button></div>}
  </>
}

function TrackMenu({ onPlaylist, onQueue }: { onPlaylist: () => void; onQueue: () => void }) {
  const [open, setOpen] = useState(false)
  return <div className="track-menu"><button className="row-icon" onClick={() => setOpen((value) => !value)} aria-label="Más acciones"><Ellipsis size={17} /></button>{open && <><button className="menu-dismiss" onClick={() => setOpen(false)} aria-label="Cerrar menú" /><div className="track-menu-pop"><button onClick={() => { onQueue(); setOpen(false) }}><Plus size={14} /> Añadir a la cola</button><button onClick={() => { onPlaylist(); setOpen(false) }}><ListMusic size={14} /> Añadir a playlist</button></div></>}</div>
}

function AlbumPage({ albums, onPlay }: { albums: AlbumGroup[]; onPlay: (album: AlbumGroup) => void }) {
  return <><PageHeading title="Álbumes" subtitle="Tu colección, ordenada por arte y artista." />{albums.length ? <div className="album-grid">{albums.map((album, index) => <button className="album-card" key={album.key} onClick={() => onPlay(album)}><span className="album-card-art" style={coverStyle(album.tracks[0], index)}>{album.tracks[0]?.coverUrl ? null : <Disc3 size={45} strokeWidth={1.15} />}</span><span className="album-card-meta"><strong>{album.name}</strong><small>{album.artist}</small><span>{album.tracks.length} {album.tracks.length === 1 ? 'canción' : 'canciones'}</span></span><span className="card-play"><Play size={15} fill="currentColor" /></span></button>)}</div> : <div className="empty-state"><div className="empty-icon"><Disc3 size={23} /></div><h2>Aún no hay álbumes.</h2><p>Los álbumes aparecen a partir de los metadatos de tu música.</p></div>}</>
}

function ArtistPage({ artists, tracksByArtist, onPlay }: { artists: string[]; tracksByArtist: Map<string, Track[]>; onPlay: (tracks: Track[]) => void }) {
  return <><PageHeading title="Artistas" subtitle="Encuentra rápido a quien quieres escuchar." />{artists.length ? <div className="artist-grid">{artists.map((artist, index) => { const songs = tracksByArtist.get(artist) ?? []; return <button className="artist-card" key={artist} onClick={() => onPlay(songs)}><span className={`artist-avatar avatar-${index % 5}`}><Mic2 size={24} strokeWidth={1.3} /></span><strong>{artist}</strong><small>{songs.length} canciones</small><span className="artist-play"><Play size={14} fill="currentColor" /></span></button> })}</div> : <div className="empty-state"><div className="empty-icon"><Mic2 size={23} /></div><h2>Todavía no hay artistas.</h2><p>Al añadir música, organizaremos aquí los artistas.</p></div>}</>
}

function PlaylistPage({ playlists, onOpen, onCreate }: { playlists: AppState['playlists']; onOpen: (id: string) => void; onCreate: () => void }) {
  return <><PageHeading title="Tus playlists" subtitle="Guarda tus canciones para volver a ellas cuando quieras." action={<button className="primary-button compact-button" onClick={onCreate}><Plus size={16} /> Nueva playlist</button>} />{playlists.length ? <div className="playlist-grid">{playlists.map((playlist, index) => <button className="playlist-card" key={playlist.id} onClick={() => onOpen(playlist.id)}><span className={`playlist-art playlist-art-${index % 4}`}><ListMusic size={27} strokeWidth={1.25} /></span><strong>{playlist.name}</strong><small>{playlist.trackCount} {playlist.trackCount === 1 ? 'canción' : 'canciones'}</small><span className="playlist-open"><ArrowUpRight size={15} /></span></button>)}</div> : <div className="empty-state"><div className="empty-icon"><ListMusic size={23} /></div><h2>Tu primera playlist empieza aquí.</h2><p>Junta canciones con una historia en común.</p><button className="secondary-button" onClick={onCreate}><Plus size={15} /> Crear playlist</button></div>}</>
}

function PlaylistDetail({ playlist, tracks, currentPath, isPlaying, onPlay, onFavorite, onQueue, onPlaylist, onRefresh }: { playlist?: AppState['playlists'][number]; tracks: Track[]; currentPath: string; isPlaying: boolean; onPlay: (track: Track) => void; onFavorite: (track: Track) => void; onQueue: (track: Track) => void; onPlaylist: (track: Track) => void; onRefresh: () => void }) {
  return <><PageHeading title={playlist?.name || 'Playlist'} subtitle={`${tracks.length} canciones · guardada en este equipo.`} action={<button className="primary-button compact-button" disabled={!tracks.length} onClick={() => tracks[0] && onPlay(tracks[0])}><Play size={15} fill="currentColor" /> Reproducir</button>} />{tracks.length ? <TrackPage title={playlist?.name || 'Playlist'} tracks={tracks} currentPath={currentPath} isPlaying={isPlaying} onPlay={onPlay} onFavorite={onFavorite} onQueue={onQueue} onPlaylist={onPlaylist} onRefresh={onRefresh} onChooseFolders={() => {}} scanning={false} showHeading={false} /> : <div className="empty-state"><div className="empty-icon"><ListMusic size={23} /></div><h2>Esta playlist está lista para llenarse.</h2><p>Desde Canciones, abre el menú de acciones y elige “Añadir a playlist”.</p></div>}</>
}

function QueuePage({ tracks, currentPath, onPlay, onClose }: { tracks: Track[]; currentPath: string; onPlay: (track: Track) => void; onClose?: () => void }) {
  return <div className={onClose ? 'queue-panel' : ''}><div className="queue-heading"><h2>Cola de reproducción</h2>{onClose && <button className="icon-button" onClick={onClose} aria-label="Cerrar cola"><X size={17} /></button>}</div><p className="queue-caption">{tracks.length} canciones · solo para esta sesión</p>{tracks.length ? <div className="queue-list">{tracks.map((track, index) => <button className={`queue-row ${track.path === currentPath ? 'current' : ''}`} key={`${track.id}-${index}`} onClick={() => onPlay(track)}><span className="queue-index">{String(index + 1).padStart(2, '0')}</span><span className="queue-cover" style={coverStyle(track, index)}>{track.coverUrl ? null : <Music2 size={14} />}</span><span><strong>{track.title}</strong><small>{track.artist}</small></span><span className="queue-duration">{formatDuration(track.durationSeconds)}</span></button>)}</div> : <div className="queue-empty"><span className="empty-icon"><Shuffle size={21} /></span><p>Reproduce una canción para empezar a llenar tu cola.</p></div>}</div>
}

function SettingsPage({ state, activeTheme, editor, imagePreview, update, updateRunning, onUpdate, onTheme, onEdit, onSave, onImage, onAudio, onChooseFolders, onRemoveFolder, onRefresh }: { state: AppState; activeTheme: Theme; editor: Theme; imagePreview: string; update: UpdateInfo; updateRunning: boolean; onUpdate: () => void; onTheme: (id: string) => void; onEdit: (theme: Theme) => void; onSave: () => void; onImage: (file?: File) => void; onAudio: (mode: string, deviceID: string, enabled: boolean, bands: number[]) => void; onChooseFolders: () => void; onRemoveFolder: (path: string) => void; onRefresh: () => void }) {
  const [name, setName] = useState(editor.name || 'Mi tema')
  const playback = usePlayback((current) => current)
  const outputMode = playback.outputMode || 'shared'
  const deviceID = playback.deviceID || ''
  const [eqEnabled, setEqEnabled] = useState(playback.equalizerOn)
  const [bands, setBands] = useState<number[]>(playback.equalizer.length === 3 ? playback.equalizer : [0, 0, 0])
  const patchTheme = (key: keyof Theme, value: string) => onEdit({ ...editor, [key]: value })
  const updateBusy = updateRunning
  const updateActionLabel = update.phase === 'checking' ? 'Buscando…'
    : update.phase === 'downloading' ? 'Descargando…'
      : update.phase === 'verifying' ? 'Verificando…'
        : update.phase === 'installing' || update.phase === 'ready' ? 'Preparando…'
          : update.phase === 'restarting' ? 'Reiniciando…'
            : update.phase === 'error' ? 'Intentar de nuevo'
              : update.phase === 'up-to-date' ? 'Buscar de nuevo'
                : update.phase === 'available' ? `Actualizar${update.version ? ` a v${update.version}` : ''}`
                  : 'Actualizar'
  const updatePercent = update.total > 0 ? Math.min(100, Math.floor((update.written / update.total) * 100)) : 0
  const updateMessage: Record<UpdatePhase, string> = {
    idle: 'Busca una versión nueva cuando quieras.',
    checking: 'Buscando una versión nueva…',
    'up-to-date': `Música Nativa está al día · v${update.currentVersion}`,
    available: `Hay una versión nueva${update.version ? ` · v${update.version}` : ''}. Al actualizar, la app se reinicia sola.`,
    downloading: 'Descargando la actualización…',
    verifying: 'Verificando el archivo descargado…',
    installing: 'Preparando el reemplazo…',
    ready: 'Preparando el reinicio…',
    restarting: 'Reiniciando Música Nativa con la versión nueva…',
    error: update.message || 'No se pudo completar la actualización.',
  }
  return <><PageHeading title="Ajustes" subtitle="Dale a tu biblioteca el ambiente que quieras y ajusta cómo suena." />
    <section className="settings-section"><div className="settings-section-title"><span className="settings-icon"><Sparkles size={18} /></span><div><h2>Temas de reproducción</h2><p>Aplica un ambiente a toda la aplicación.</p></div><span className="settings-tag">{state.themes.length} TEMAS</span></div><div className="theme-grid">{state.themes.map((theme, index) => <div key={theme.id} className={`theme-card ${activeTheme.id === theme.id ? 'selected' : ''}`}><button className={`theme-preview theme-preview-${index % 4}`} style={{ background: theme.background }} onClick={() => onTheme(theme.id)}><span className="theme-preview-window" style={{ background: theme.surface }}><i style={{ background: theme.accent }} /><i /><i /></span>{theme.backgroundImage && <span className="theme-photo" style={{ backgroundImage: `url("${theme.backgroundImage}")` }} />}{activeTheme.id === theme.id && <span className="theme-check"><Check size={14} /></span>}</button><div className="theme-card-meta"><div><strong>{theme.name}</strong><small>{theme.builtIn ? 'Incluido' : 'Creado por ti'}</small></div><button className={`apply-theme-button ${activeTheme.id === theme.id ? 'applied' : ''}`} onClick={() => onTheme(theme.id)}>{activeTheme.id === theme.id ? 'Aplicado' : 'Aplicar'}</button></div></div>)}</div></section>

    <section className="settings-section editor-section"><div className="settings-section-title"><span className="settings-icon"><SlidersHorizontal size={18} /></span><div><h2>Diseña un tema</h2><p>Escoge colores y una imagen para personalizarlo.</p></div></div><div className="theme-editor"><div className="editor-preview" style={{ background: editor.background, backgroundImage: imagePreview || editor.backgroundImage ? `linear-gradient(180deg, rgba(0,0,0,.08), ${editor.background}d9), url("${imagePreview || editor.backgroundImage}")` : undefined }}><div className="editor-cover" style={coverStyle(undefined, 0)}><Disc3 size={29} /></div><div className="editor-playing"><strong>Una forma propia</strong><small>Tu música, a tu manera</small></div><div className="editor-controls"><span /><span /><span /></div></div><div className="editor-fields"><label className="field-label">Nombre del tema<input value={name} onChange={(event) => { setName(event.target.value); patchTheme('name', event.target.value) }} placeholder="Mi tema" maxLength={32} /></label><div className="color-fields"><ColorField label="Fondo" value={editor.background} onChange={(value) => patchTheme('background', value)} /><ColorField label="Acento" value={editor.accent} onChange={(value) => patchTheme('accent', value)} /><ColorField label="Superficie" value={editor.surface} onChange={(value) => patchTheme('surface', value)} /><ColorField label="Texto" value={editor.text} onChange={(value) => patchTheme('text', value)} /></div><label className="image-upload"><input type="file" accept="image/png,image/jpeg,image/webp" onChange={(event) => onImage(event.target.files?.[0])} /><span className="upload-icon"><FolderPlus size={16} /></span><span><strong>Usar imagen de fondo</strong><small>JPG, PNG o WebP · máximo 8 MB</small></span><ArrowUpRight size={15} /></label><button className="primary-button save-theme-button" onClick={onSave}><Check size={15} /> Guardar tema</button></div></div></section>

    <section className="settings-section updates-section"><div className="settings-section-title"><span className="settings-icon"><RefreshCw size={18} /></span><div><h2>Actualizaciones</h2><p>Un botón instala la versión nueva y reinicia la app.</p></div><span className="settings-tag">VERSIÓN {state.version}</span></div><div className="update-panel"><div className={`update-indicator ${update.phase}`}><span /></div><div className="update-copy"><strong>{update.name || 'Música Nativa'}</strong><p role="status">{updateMessage[update.phase]}</p>{update.notes && (update.phase === 'available' || update.phase === 'ready' || update.phase === 'restarting') && <pre className="update-notes">{update.notes}</pre>}{update.phase === 'downloading' && <div className="update-progress" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={updatePercent}><span style={{ width: `${updatePercent}%` }} /></div>}</div><div className="update-actions"><button className={`${update.phase === 'available' ? 'primary-button' : 'secondary-button'} compact-button`} onClick={onUpdate} disabled={updateBusy}>{updateBusy ? <span className="update-spinner" /> : <RefreshCw size={14} />}{updateActionLabel}</button></div></div></section>

    <section className="settings-section"><div className="settings-section-title"><span className="settings-icon"><AudioLines size={18} /></span><div><h2>Audio</h2><p>Elige la salida y ajusta el ecualizador opcional.</p></div><span className={`output-status ${playback.outputReady ? 'ready' : ''}`}><i />{playback.outputReady ? 'DISPOSITIVO LISTO' : 'AUDIO SIN CONECTAR'}</span></div><div className="audio-settings"><div className="output-setting"><div><strong>Modo de salida</strong><small>Compartido funciona con otras aplicaciones. Exclusivo intenta reservar el dispositivo.</small></div><div className="mode-switch"><button className={outputMode === 'shared' ? 'active' : ''} onClick={() => onAudio('shared', deviceID, eqEnabled, bands)}>Compartido</button><button className={outputMode === 'exclusive' ? 'active' : ''} onClick={() => onAudio('exclusive', deviceID, eqEnabled, bands)}>Exclusivo</button></div></div><div className="output-setting device-setting"><div><strong>Dispositivo de salida</strong><small>Predeterminado sigue el dispositivo de audio elegido en Windows.</small></div><select aria-label="Dispositivo de salida" className="device-select" value={deviceID} onChange={(event) => onAudio(outputMode, event.target.value, eqEnabled, bands)}><option value="">Predeterminado de Windows</option>{state.outputDevices.map((device) => <option key={device.id} value={device.id}>{device.name}{device.default ? ' · predeterminado' : ''}</option>)}</select></div><div className="eq-setting"><div className="eq-title"><div><strong>Ecualizador de 3 bandas</strong><small>El ajuste predeterminado conserva el sonido original.</small></div><button role="switch" aria-checked={eqEnabled} className={`toggle-switch ${eqEnabled ? 'on' : ''}`} onClick={() => { const next = !eqEnabled; setEqEnabled(next); onAudio(outputMode, deviceID, next, bands) }}><span /></button></div><div className={`eq-bands ${eqEnabled ? '' : 'disabled'}`}>{['Graves', 'Medios', 'Agudos'].map((label, index) => <label className="eq-band" key={label}><span>{label}<small>{bands[index] > 0 ? '+' : ''}{bands[index]} dB</small></span><input type="range" min={-12} max={12} step={1} value={bands[index]} disabled={!eqEnabled} onChange={(event) => { const next = [...bands]; next[index] = Number(event.target.value); setBands(next); onAudio(outputMode, deviceID, eqEnabled, next) }} style={{ accentColor: activeTheme.accent }} /><small>{index === 0 ? '100 Hz' : index === 1 ? '1 kHz' : '10 kHz'}</small></label>)}</div></div>{playback.error && <p className="audio-message"><span />{playback.error}</p>}</div></section>

    <section className="settings-section folder-settings"><div className="settings-section-title"><span className="settings-icon"><Library size={18} /></span><div><h2>Carpetas de música</h2><p>Solo se leen metadatos: nunca copiamos ni modificamos tus canciones.</p></div><button className="text-button" onClick={onRefresh}><RefreshCw size={14} /> Actualizar</button></div><div className="folder-list">{state.folders.length ? state.folders.map((folder) => <div className="folder-row" key={folder}><span className="folder-mini-icon"><Library size={16} /></span><span><strong>{folder.split(/[\\/]/).pop() || folder}</strong><small>{folder}</small></span><button className="row-icon" onClick={() => onRemoveFolder(folder)} aria-label={`Quitar ${folder}`}><Trash2 size={15} /></button></div>) : <p className="folder-empty">Aún no has añadido carpetas.</p>}<button className="add-folder-row" onClick={onChooseFolders}><Plus size={15} /> Añadir carpeta</button></div></section>
    <div className="settings-footer"><span>VERSIÓN {state.version} · MÚSICA NATIVA</span><span>Archivos tuyos. Escucha tuya.</span></div>
  </>
}

function ColorField({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  const hex = /^#[0-9a-f]{6}$/i.test(value) ? value : '#11110f'
  return <label className="color-field"><span>{label}</span><span className="color-picker-wrap"><input type="color" value={hex} onChange={(event) => onChange(event.target.value)} /><i style={{ background: hex }} /></span><code>{value}</code></label>
}

function PlayerBar({ track, onToggle, onNext, onPrevious, onSeek, onVolume, onFavorite, onQueue, onSettings, panelOpen, onTogglePanel }: { track?: Track; onToggle: () => void; onNext: () => void; onPrevious: () => void; onSeek: (value: number) => Promise<unknown>; onVolume: (value: number) => void; onFavorite?: () => void; onQueue: () => void; onSettings: () => void; panelOpen: boolean; onTogglePanel: () => void }) {
  const playback = usePlayback((current) => current)
  return <footer className="player-bar"><div className="now-playing"><span className="player-cover" style={coverStyle(track, 0)}>{track?.coverUrl ? null : <Disc3 size={22} strokeWidth={1.2} />}</span><span className="now-playing-text"><strong>{track?.title || 'Elige lo que quieres escuchar'}</strong><small>{track?.artist || 'Tu biblioteca local'}</small></span>{onFavorite && <button className={`player-heart ${track?.favorite ? 'liked' : ''}`} onClick={onFavorite} aria-label="Marcar como favorito"><Heart size={16} fill={track?.favorite ? 'currentColor' : 'none'} /></button>}</div><div className="player-center"><div className="transport-controls"><button className="transport-subtle" onClick={onPrevious} aria-label="Canción anterior"><SkipBack size={19} fill="currentColor" /></button><button className="play-button" onClick={onToggle} aria-label={playback.isPlaying ? 'Pausar' : 'Reproducir'}>{playback.isPlaying ? <Pause size={18} fill="currentColor" /> : <Play size={18} fill="currentColor" className="play-icon" />}</button><button className="transport-subtle" onClick={onNext} aria-label="Siguiente canción"><SkipForward size={19} fill="currentColor" /></button></div><SeekBar positionMs={playback.positionMs} durationMs={playback.durationMs} disabled={!playback.path} onSeek={onSeek} /></div><div className="player-tools"><button className={`tool-button panel-toggle ${panelOpen ? 'active' : ''}`} onClick={onTogglePanel} aria-pressed={panelOpen} aria-label="Panel Reproduciendo" title="Panel Reproduciendo"><PanelRight size={17} /></button><button className="tool-button" onClick={onQueue} aria-label="Ver cola" title="Cola"><ListMusic size={17} /></button><button className="tool-button" onClick={onSettings} aria-label="Ajustes de audio"><SlidersHorizontal size={17} /></button><Volume2 size={17} className="volume-icon" /><input aria-label="Volumen" className="volume-range" type="range" min={0} max={100} value={playback.volume} onChange={(event) => onVolume(Number(event.target.value))} style={{ accentColor: 'var(--accent)' }} /><span className={`player-output-dot ${playback.outputReady ? 'ready' : ''}`} title={playback.outputReady ? `Audio ${playback.outputMode}` : 'Sin salida de audio'} /></div></footer>
}

// SeekBar follows the pointer while the listener drags and seeks once, when the
// slider is released: every seek reopens and decodes the file again.
function SeekBar({ positionMs, durationMs, disabled, onSeek }: { positionMs: number; durationMs: number; disabled: boolean; onSeek: (value: number) => Promise<unknown> }) {
  const input = useRef<HTMLInputElement>(null)
  const [draggedMs, setDraggedMs] = useState<number | null>(null)
  const seek = useRef(onSeek)
  useEffect(() => { seek.current = onSeek })
  useEffect(() => {
    const element = input.current
    if (!element) return
    // The native change event fires when the value is committed (pointer
    // released or key pressed); React's onChange fires on every movement.
    const commit = () => { void seek.current(Number(element.value)).finally(() => setDraggedMs(null)) }
    element.addEventListener('change', commit)
    return () => element.removeEventListener('change', commit)
  }, [])
  const shownMs = draggedMs ?? Math.min(positionMs, durationMs || 1)
  return <div className="seek-row"><span>{formatTime(shownMs)}</span><input ref={input} aria-label="Posición de la canción" type="range" min={0} max={Math.max(1, durationMs)} value={shownMs} onChange={(event) => setDraggedMs(Number(event.target.value))} style={{ accentColor: 'var(--accent)' }} disabled={disabled} /><span>{formatTime(durationMs)}</span></div>
}

// ScanStatus shows scan progress. It listens on its own, so progress messages
// re-render this line only.
function ScanStatus({ onViewLibrary }: { onViewLibrary: () => void }) {
  const [scan, setScan] = useState<ScanProgress | null>(null)
  useEffect(() => Events.On('library:scan-progress', (event) => setScan(event.data as ScanProgress)), [])
  if (scan?.phase !== 'scanning') return null
  return <div className="scan-status"><span className="scan-spinner" />{scan.message || 'Buscando música…'}<button onClick={onViewLibrary}>Ver biblioteca</button></div>
}

export default App

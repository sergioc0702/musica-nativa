import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type FormEvent, type ReactNode } from 'react'
import { AnimatePresence, motion, useReducedMotion } from 'motion/react'
import { Events } from '@wailsio/runtime'
import {
  AudioLines, Check, ChevronDown, ChevronUp, Clock3, Info, PanelLeftClose, PanelLeftOpen, Palette, Disc3, Ellipsis, Heart, Keyboard, MicVocal, PanelRight, Repeat, Repeat1,
  House, Library, ListMusic, ListOrdered, Music2, Pause, Play, Plus, Search, Settings, UserRound,
  Shuffle, SkipBack, SkipForward, SlidersVertical, TriangleAlert, Volume1, Volume2, VolumeX, X,
  FolderPlus, RefreshCw, ArrowUpRight, Trash2,
} from 'lucide-react'
import { LibraryService } from '../bindings/musica-nativa'
import type { AppState, ListeningHistory, PlaybackState, PlayMode, ScanProgress, Theme } from '../bindings/musica-nativa/models'
import type { Track } from '../bindings/musica-nativa/internal/library/models'
import '../bindings/events'
import { getPlayback, setPlayback, usePlayback } from './playback'
import { albumKey, groupAlbums, nameKey, type AlbumGroup } from './albums'
import { coverStyle, formatDuration, formatTime } from './format'
import { HomePage } from './Home'
import { AlbumView, AlbumsPage } from './AlbumPages'
import { ArtistView, ArtistsPage } from './ArtistPages'
import { FavoritesView } from './FavoritesPage'
import { FloatingPanel } from './FloatingPanel'
import { useCoverColor } from './coverColor'
import { raisePanel, togglePanel, type PanelID, type Rect, type Size } from './floating'
import { artistAlbums, topArtists, type ArtistSummary } from './homeSections'
import { formatLength, sortNames, sortTracks, type SortKey } from './trackSort'
import { EqualizerPanel } from './EqualizerPanel'
import { LyricsView } from './LyricsView'

const panelTitles: Record<PanelID, string> = { equalizer: 'Ecualizador', queue: 'Cola', lyrics: 'Letra' }
const panelIcons: Record<PanelID, ReactNode> = {
  equalizer: <SlidersVertical size={15} />,
  queue: <ListOrdered size={15} />,
  lyrics: <MicVocal size={15} />,
}
const panelMin: Record<PanelID, Size> = {
  equalizer: { width: 520, height: 360 },
  queue: { width: 300, height: 240 },
  lyrics: { width: 320, height: 280 },
}
// Where each panel opens the first time: the equalizer above the player bar,
// the queue on the right and the lyrics on the left.
const panelStart: Record<PanelID, (window: Size) => Rect> = {
  equalizer: (window) => ({ x: window.width - 690, y: window.height - 580, width: 660, height: 470 }),
  queue: (window) => ({ x: window.width - 440, y: 84, width: 400, height: Math.min(560, window.height - 220) }),
  lyrics: (window) => ({ x: 260, y: 84, width: 520, height: Math.min(620, window.height - 220) }),
}

type SettingsTab = 'library' | 'sound' | 'look' | 'lyrics' | 'keys' | 'about'
const settingsTabs: { id: SettingsTab; label: string; icon: typeof Library }[] = [
  { id: 'library', label: 'Biblioteca', icon: Library },
  { id: 'sound', label: 'Sonido', icon: AudioLines },
  { id: 'look', label: 'Apariencia', icon: Palette },
  { id: 'lyrics', label: 'Letras', icon: MicVocal },
  { id: 'keys', label: 'Atajos', icon: Keyboard },
  { id: 'about', label: 'Acerca de', icon: Info },
]

const shortcutList: [string[], string][] = [
  [['Espacio'], 'Reproducir o pausar'],
  [['Ctrl', '→'], 'Siguiente canción'],
  [['Ctrl', '←'], 'Canción anterior'],
  [['→'], 'Adelantar 5 segundos'],
  [['←'], 'Retroceder 5 segundos'],
  [['Ctrl', '↑'], 'Subir el volumen'],
  [['Ctrl', '↓'], 'Bajar el volumen'],
  [['S'], 'Aleatorio'],
  [['R'], 'Repetir: no, toda la cola o esta canción'],
  [['E'], 'Abrir o cerrar el ecualizador'],
  [['L'], 'Mostrar u ocultar la letra'],
  [['C'], 'Mostrar u ocultar la cola'],
  [['Ctrl', 'K'], 'Buscar'],
]

type View = 'home' | 'tracks' | 'albums' | 'artists' | 'favorites' | 'playlists' | 'settings' | 'playlist' | 'album' | 'artist'
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

// BrandLogo is Música Nativa's mark: three sound bars on the accent color.
function BrandLogo() {
  return <svg className="brand-logo" viewBox="0 0 32 32" aria-hidden="true">
    <defs>
      <linearGradient id="brand-logo-fill" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0" style={{ stopColor: 'var(--accent)' }} />
        <stop offset="1" style={{ stopColor: 'var(--accent)', stopOpacity: 0.62 }} />
      </linearGradient>
    </defs>
    <rect width="32" height="32" rx="9" fill="url(#brand-logo-fill)" />
    <circle cx="16" cy="16" r="10.5" fill="none" stroke="var(--on-accent)" strokeOpacity=".22" strokeWidth="1.4" />
    <rect x="9.6" y="13" width="3.2" height="8" rx="1.6" fill="var(--on-accent)" />
    <rect x="14.4" y="8.5" width="3.2" height="15" rx="1.6" fill="var(--on-accent)" />
    <rect x="19.2" y="11" width="3.2" height="11" rx="1.6" fill="var(--on-accent)" />
  </svg>
}

// playlistShade picks one of the cover gradients for a playlist, always the same for a name.
const playlistShade = (name: string) => [...name].reduce((total, letter) => total + letter.charCodeAt(0), 0)

const navItems: { id: View; label: string; icon: typeof House }[] = [
  { id: 'home', label: 'Inicio', icon: House },
  { id: 'tracks', label: 'Canciones', icon: Music2 },
  { id: 'albums', label: 'Álbumes', icon: Disc3 },
  { id: 'artists', label: 'Artistas', icon: UserRound },
  { id: 'favorites', label: 'Favoritos', icon: Heart },
]

function displayError(error: unknown) {
  return error instanceof Error ? error.message : String(error || 'Ocurrió un problema.')
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
  // The floating panels that are open, from the one at the back to the one in front.
  const [panels, setPanels] = useState<PanelID[]>([])
  const showQueue = panels.includes('queue')
  const showEqualizer = panels.includes('equalizer')
  const showLyrics = panels.includes('lyrics')
  const toggle = (id: PanelID) => setPanels((open) => togglePanel(open, id))
  const bringUp = (id: PanelID) => setPanels((open) => (open.includes(id) ? raisePanel(open, id) : [...open, id]))
  const [openAlbumKey, setOpenAlbumKey] = useState('')
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => {
    try { return localStorage.getItem('mn-sidebar') === 'collapsed' } catch { return false }
  })
  useEffect(() => {
    try { localStorage.setItem('mn-sidebar', sidebarCollapsed ? 'collapsed' : 'open') } catch { /* private window */ }
  }, [sidebarCollapsed])
  const [openArtistName, setOpenArtistName] = useState('')
  // Below 1100 px the side panel does not fit (style.css hides it).
  const [narrow, setNarrow] = useState(() => window.matchMedia('(max-width: 1100px)').matches)
  const [showNowPlaying, setShowNowPlaying] = useState(() => {
    try { return localStorage.getItem('mn-now-panel') !== 'off' } catch { return true }
  })
  const [playQueue, setPlayQueue] = useState<Track[]>([])
  const [playMode, setPlayMode] = useState<PlayMode>({ shuffle: false, repeat: 'off' })
  const [history, setHistory] = useState<ListeningHistory>({ recent: [], top: [], lastPlayed: {} })
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
    const listener = (event: KeyboardEvent) => handleShortcut.current(event)
    window.addEventListener('keydown', listener)
    return () => window.removeEventListener('keydown', listener)
  }, [])

  const activeTheme = useMemo(
    () => state.themes.find((theme) => theme.id === state.activeTheme) || defaultTheme,
    [state.activeTheme, state.themes],
  )
  const playbackPath = usePlayback((playback) => playback.path)
  useEffect(() => {
    try { localStorage.setItem('mn-now-panel', showNowPlaying ? 'on' : 'off') } catch { /* private window */ }
  }, [showNowPlaying])
  const loadPlayQueue = useCallback(() => {
    LibraryService.Queue().then(setPlayQueue).catch(() => setPlayQueue([]))
  }, [])
  useEffect(() => {
    if (playbackPath) loadPlayQueue()
  }, [playbackPath, queue, loadPlayQueue])
  useEffect(() => {
    LibraryService.PlayMode().then(setPlayMode).catch(() => { /* keep the defaults */ })
  }, [])
  // The backend sends the history again each time a song counts as listened.
  useEffect(() => {
    LibraryService.ListeningHistory().then(setHistory).catch(() => { /* the home page works without it */ })
    return Events.On('history:changed', (event) => setHistory(event.data as ListeningHistory))
  }, [])
  const isPlaying = usePlayback((playback) => playback.isPlaying)
  const currentTrack = playbackPath ? state.tracks.find((track) => track.path === playbackPath) ?? queue.find((track) => track.path === playbackPath) : undefined
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const searchedTracks = useMemo(() => state.tracks.filter((track) => {
    if (!normalizedQuery) return true
    return `${track.title} ${track.artist} ${track.album}`.toLocaleLowerCase().includes(normalizedQuery)
  }), [normalizedQuery, state.tracks])
  const favoriteTracks = useMemo(() => searchedTracks.filter((track) => track.favorite), [searchedTracks])
  const albums = useMemo(() => groupAlbums(state.tracks), [state.tracks])
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

  useEffect(() => {
    const query = window.matchMedia('(max-width: 1100px)')
    const onChange = () => setNarrow(query.matches)
    query.addEventListener('change', onChange)
    return () => query.removeEventListener('change', onChange)
  }, [])

  // One button shows what plays next: the side panel when the window is wide
  // enough, the floating queue when it is not.
  const togglePlayingView = () => {
    if (!narrow) {
      setShowNowPlaying((open) => !open)
      return
    }
    toggle('queue')
    loadPlayQueue()
  }

  const toggleLyrics = () => toggle('lyrics')
  const seekLyrics = useCallback((positionMs: number) => {
    void LibraryService.SeekTo(positionMs).then(setPlayback)
  }, [])

  const toggleEqualizer = () => toggle('equalizer')


  const openedAlbum = view === 'album' ? albums.find((album) => album.key === openAlbumKey) : undefined
  const openAlbum = (album: AlbumGroup) => {
    setOpenAlbumKey(album.key)
    setView('album')
  }
  const playAlbum = (album: AlbumGroup) => {
    const first = album.tracks.find((track) => track.available)
    if (first) void playTrack(first, album.tracks)
  }
  // Every artist with their songs (album by album) and how much they were heard.
  const artistSummaries = useMemo(() => topArtists(state.tracks, history.top ?? [], Infinity), [state.tracks, history.top])
  const favoriteCount = state.tracks.filter((track) => track.favorite).length
  const navCounts: Partial<Record<View, number>> = { tracks: state.tracks.length, albums: albums.length, artists: artistSummaries.length, favorites: favoriteCount }
  const openedArtist = view === 'artist' ? artistSummaries.find((artist) => artist.key === openArtistName) : undefined
  const openArtist = (name: string) => {
    setOpenArtistName(name)
    setView('artist')
  }
  const playArtist = (artist: ArtistSummary) => {
    const first = artist.tracks.find((track) => track.available)
    if (first) void playTrack(first, artist.tracks)
  }

  const resumeListening = async () => {
    try {
      await LibraryService.ResumeListening()
      await refreshQueue()
    } catch (error) { notify(displayError(error)) }
  }

  // shufflePlay turns shuffle on and plays list from a random song.
  const shufflePlay = async (list: Track[]) => {
    const playable = list.filter((track) => track.available)
    if (!playable.length) return
    try {
      if (!playMode.shuffle) setPlayMode(await LibraryService.SetShuffle(true))
      await playTrack(playable[Math.floor(Math.random() * playable.length)], playable)
    } catch (error) { notify(displayError(error)) }
  }

  const toggleShuffle = async () => {
    try {
      setPlayMode(await LibraryService.SetShuffle(!playMode.shuffle))
      loadPlayQueue()
    } catch (error) { notify(displayError(error)) }
  }

  const cycleRepeat = async () => {
    const next = playMode.repeat === 'off' ? 'all' : playMode.repeat === 'all' ? 'one' : 'off'
    try { setPlayMode(await LibraryService.SetRepeat(next)) } catch (error) { notify(displayError(error)) }
  }

  const seekBy = (deltaMs: number) => {
    const playback = getPlayback()
    if (!playback.path) return
    const target = Math.min(Math.max(playback.positionMs + deltaMs, 0), playback.durationMs || Number.MAX_SAFE_INTEGER)
    void LibraryService.SeekTo(target).then(setPlayback)
  }

  const changeVolumeBy = (delta: number) => {
    const volume = Math.min(Math.max(getPlayback().volume + delta, 0), 100)
    void LibraryService.SetVolume(volume).then(setPlayback)
  }

  // The listener is added once; it always calls the latest handler.
  const handleShortcut = useRef<(event: KeyboardEvent) => void>(() => {})
  handleShortcut.current = (event) => {
    const command = event.ctrlKey || event.metaKey
    if (command && event.key.toLowerCase() === 'k') {
      event.preventDefault()
      searchInput.current?.focus()
      return
    }
    const target = event.target instanceof HTMLElement ? event.target : null
    if (event.altKey || target?.closest('input, textarea, select, [contenteditable="true"]')) return
    if (showPlaylistForm || playlistTarget) return
    const key = event.key.toLowerCase()
    if (event.code === 'Space' && !command) {
      event.preventDefault()
      if (!event.repeat) void togglePlay()
    } else if (key === 'arrowright' || key === 'arrowleft') {
      event.preventDefault()
      const forward = key === 'arrowright'
      if (command) { if (!event.repeat) void (forward ? playNext() : playPrevious()) }
      else seekBy(forward ? 5000 : -5000)
    } else if (command && (key === 'arrowup' || key === 'arrowdown')) {
      event.preventDefault()
      changeVolumeBy(key === 'arrowup' ? 5 : -5)
    } else if (!command && !event.repeat && key === 's') {
      void toggleShuffle()
    } else if (!command && !event.repeat && key === 'r') {
      void cycleRepeat()
    } else if (!command && !event.repeat && key === 'e') {
      toggleEqualizer()
    } else if (!command && !event.repeat && key === 'l') {
      toggleLyrics()
    } else if (!command && !event.repeat && key === 'c') {
      toggle('queue')
      loadPlayQueue()
    }
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

  const setAudioOptions = async (mode: string, deviceID: string) => {
    try {
      const next = await LibraryService.SetAudioOptions(mode, deviceID)
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
  const title = view === 'home' ? 'Inicio' : view === 'tracks' ? 'Canciones' : view === 'albums' ? 'Álbumes' : view === 'artists' ? 'Artistas' : view === 'favorites' ? 'Tus favoritos' : view === 'settings' ? 'Ajustes' : view === 'playlist' ? selectedPlaylistItem?.name || 'Playlist' : 'Tus playlists'

  return (
    <div className={`app-shell ${showNowPlaying && currentTrack ? 'with-panel' : ''} ${sidebarCollapsed ? 'sidebar-collapsed' : ''}`} style={themeVariables}>
      {activeTheme.backgroundImage && <div className="theme-backdrop" style={{ backgroundImage: `linear-gradient(90deg, var(--app-bg) 0%, color-mix(in srgb, var(--app-bg) 86%, transparent) 58%, color-mix(in srgb, var(--app-bg) 66%, transparent) 100%), linear-gradient(180deg, color-mix(in srgb, var(--app-bg) 72%, transparent), var(--app-bg)), url("${activeTheme.backgroundImage}")` }} />}
      <aside className="sidebar">
        <div className="sidebar-top">
          <button className="brand" onClick={() => setView('home')} aria-label="Música Nativa, inicio">
            <BrandLogo />
            <span className="brand-wordmark">música<span>nativa</span></span>
          </button>
          <button className="icon-button sidebar-collapse" onClick={() => setSidebarCollapsed((current) => !current)} aria-label={sidebarCollapsed ? 'Mostrar el menú completo' : 'Dejar solo los iconos'} title={sidebarCollapsed ? 'Mostrar el menú completo' : 'Dejar solo los iconos'}>{sidebarCollapsed ? <PanelLeftOpen size={17} /> : <PanelLeftClose size={17} />}</button>
        </div>

        <div className="sidebar-section-heading"><span>Tu biblioteca</span></div>
        <nav className="primary-nav" aria-label="Biblioteca">
          {navItems.map(({ id, label, icon: Icon }) => {
            const active = view === id || (id === 'albums' && view === 'album') || (id === 'artists' && view === 'artist')
            const count = navCounts[id]
            return <button key={id} className={`nav-item ${active ? 'active' : ''}`} aria-label={count ? `${label}, ${count}` : label} aria-current={active ? 'page' : undefined} title={label} onClick={() => setView(id)}><span className="nav-chip"><Icon size={17} strokeWidth={1.9} /></span><span className="nav-label">{label}</span>{count ? <small>{count}</small> : null}</button>
          })}
        </nav>

        <div className="sidebar-section-heading"><span>Playlists</span><button className="icon-button tiny" onClick={() => setShowPlaylistForm(true)} aria-label="Crear playlist" title="Crear playlist"><Plus size={16} /></button></div>
        <nav className="playlist-nav" aria-label="Playlists">
          {state.playlists.map((playlist) => {
            const active = view === 'playlist' && selectedPlaylist === playlist.id
            const covers = playlist.covers ?? []
            return <button key={playlist.id} className={`nav-item playlist-link ${active ? 'active' : ''}`} aria-label={`Playlist ${playlist.name}, ${playlist.trackCount} ${playlist.trackCount === 1 ? 'canción' : 'canciones'}`} aria-current={active ? 'page' : undefined} title={playlist.name} onClick={() => void loadPlaylist(playlist.id)}>
              <span className={`nav-playlist-art ${covers.length >= 4 ? 'four' : ''}`} style={covers.length ? undefined : coverStyle(undefined, playlistShade(playlist.name))}>
                {covers.length ? (covers.length >= 4 ? covers : covers.slice(0, 1)).map((cover) => <span key={cover} style={{ backgroundImage: `url("${cover}")` }} />) : playlist.name.trim().charAt(0).toLocaleUpperCase() || '♪'}
              </span>
              <span className="nav-label playlist-text"><strong>{playlist.name}</strong><small>{playlist.trackCount} {playlist.trackCount === 1 ? 'canción' : 'canciones'}</small></span>
            </button>
          })}
          {!state.playlists.length && <button className="sidebar-create" onClick={() => setShowPlaylistForm(true)} title="Crear playlist"><span className="nav-chip"><Plus size={16} /></span><span className="nav-label">Crea tu primera playlist</span></button>}
        </nav>

        <div className="sidebar-bottom">
          <button className={`nav-item settings-link ${view === 'settings' ? 'active' : ''}`} aria-label={update.phase === 'available' ? 'Ajustes, hay una versión nueva' : 'Ajustes'} aria-current={view === 'settings' ? 'page' : undefined} title={update.phase === 'available' ? 'Ajustes · versión nueva disponible' : 'Ajustes'} onClick={() => setView('settings')}><span className="nav-chip"><Settings size={17} strokeWidth={1.9} /></span><span className="nav-label">Ajustes</span>{update.phase === 'available' && <i className="update-dot" aria-hidden="true" />}</button>
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
              {view === 'home' && <HomePage tracks={state.tracks} albums={albums} history={history} currentTrack={currentTrack} scanning={state.scanning} onPlay={(track, list) => void playTrack(track, list)} onPlayAlbum={playAlbum} onOpenAlbum={openAlbum} onOpenArtist={openArtist} onToggle={() => void togglePlay()} onResume={() => void resumeListening()} onShuffleAll={() => void shufflePlay(state.tracks)} onFavorites={() => void shufflePlay(state.tracks.filter((track) => track.favorite))} onBrowse={() => void chooseFolders()} onViewAlbums={() => setView('albums')} />}
              {view === 'tracks' && <TrackPage manageLibrary title={title} tracks={visibleTracks} currentPath={playbackPath} isPlaying={isPlaying} onPlay={(track, list) => void playTrack(track, list)} onShuffle={(list) => void shufflePlay(list)} onFavorite={(track) => void toggleFavorite(track)} onQueue={(track) => void addToQueue(track)} onPlaylist={(track) => void addToPlaylist(track)} onRefresh={() => void startScan()} onChooseFolders={() => void chooseFolders()} scanning={state.scanning} />}
              {view === 'favorites' && <FavoritesView tracks={visibleTracks} searching={!!normalizedQuery} onPlay={() => { const first = visibleTracks.find((track) => track.available); if (first) void playTrack(first, visibleTracks) }} onShuffle={() => void shufflePlay(visibleTracks)} onBrowse={() => setView('tracks')}><TrackPage title={title} tracks={visibleTracks} currentPath={playbackPath} isPlaying={isPlaying} onPlay={(track, list) => void playTrack(track, list)} onFavorite={(track) => void toggleFavorite(track)} onQueue={(track) => void addToQueue(track)} onPlaylist={(track) => void addToPlaylist(track)} onRefresh={() => {}} onChooseFolders={() => {}} scanning={false} showHeading={false} toolbar={false} /></FavoritesView>}
              {view === 'albums' && <AlbumsPage albums={albums.filter((album) => !normalizedQuery || `${album.name} ${album.artist}`.toLocaleLowerCase().includes(normalizedQuery))} onOpen={openAlbum} onPlay={playAlbum} />}
              {view === 'album' && openedAlbum && <AlbumView album={openedAlbum} onBack={() => setView('albums')} onPlay={() => playAlbum(openedAlbum)} onShuffle={() => void shufflePlay(openedAlbum.tracks)}><TrackPage title={openedAlbum.name} tracks={openedAlbum.tracks} currentPath={playbackPath} isPlaying={isPlaying} onPlay={(track, list) => void playTrack(track, list)} onFavorite={(track) => void toggleFavorite(track)} onQueue={(track) => void addToQueue(track)} onPlaylist={(track) => void addToPlaylist(track)} onRefresh={() => {}} onChooseFolders={() => {}} scanning={false} showHeading={false} toolbar={false} albumView defaultSort="order" /></AlbumView>}
              {view === 'artists' && <ArtistsPage artists={artistSummaries.filter((artist) => !normalizedQuery || artist.name.toLocaleLowerCase().includes(normalizedQuery))} onOpen={(artist) => openArtist(artist.key)} onPlay={playArtist} />}
              {view === 'artist' && openedArtist && <ArtistView artist={openedArtist} albums={artistAlbums(openedArtist.key, albums)} onBack={() => setView('artists')} onPlay={() => playArtist(openedArtist)} onShuffle={() => void shufflePlay(openedArtist.tracks)} onOpenAlbum={openAlbum} onPlayAlbum={playAlbum}><TrackPage title={openedArtist.name} tracks={openedArtist.tracks} currentPath={playbackPath} isPlaying={isPlaying} onPlay={(track, list) => void playTrack(track, list)} onFavorite={(track) => void toggleFavorite(track)} onQueue={(track) => void addToQueue(track)} onPlaylist={(track) => void addToPlaylist(track)} onRefresh={() => {}} onChooseFolders={() => {}} scanning={false} showHeading={false} toolbar={false} defaultSort="order" /></ArtistView>}
              {view === 'playlists' && <PlaylistPage playlists={state.playlists} onOpen={(id) => void loadPlaylist(id)} onCreate={() => setShowPlaylistForm(true)} />}
              {view === 'playlist' && <PlaylistDetail playlist={selectedPlaylistItem} tracks={queue} currentPath={playbackPath} isPlaying={isPlaying} onPlay={(track) => void playTrack(track, queue)} onFavorite={(track) => void toggleFavorite(track)} onQueue={(track) => void addToQueue(track)} onPlaylist={(track) => void addToPlaylist(track)} onRefresh={refreshQueue} />}
              {view === 'settings' && <SettingsPage state={state} activeTheme={activeTheme} editor={editor} imagePreview={imagePreview} update={update} updateRunning={updateRunning} onUpdate={() => void updateNow()} onTheme={(theme) => void selectTheme(theme)} onEdit={setEditor} onSave={() => void saveTheme()} onImage={readBackground} onAudio={setAudioOptions} onEqualizer={() => bringUp('equalizer')} onChooseFolders={() => void chooseFolders()} onRemoveFolder={async (path) => { try { await LibraryService.RemoveMusicFolder(path); await refreshState() } catch (error) { notify(displayError(error)) } }} onRefresh={() => void startScan()} />}
            </motion.section>
          </AnimatePresence>
        </div>

      </main>

      {showNowPlaying && currentTrack && <NowPlayingPanel onShowQueue={() => { bringUp('queue'); loadPlayQueue() }} track={currentTrack} queue={playQueue} onPlay={(track) => void playTrack(track, playQueue)} onFavorite={currentTrack ? () => void toggleFavorite(currentTrack) : undefined} onClose={() => setShowNowPlaying(false)} />}
      <PlayerBar track={currentTrack} onToggle={() => void togglePlay()} onNext={() => void playNext()} onPrevious={() => void playPrevious()} onSeek={(value) => LibraryService.SeekTo(value).then(setPlayback)} onVolume={(value) => { void LibraryService.SetVolume(value).then(setPlayback) }} onFavorite={currentTrack ? () => void toggleFavorite(currentTrack) : undefined} lyricsOpen={showLyrics} onLyrics={toggleLyrics} eqOpen={showEqualizer} onEqualizer={toggleEqualizer} panelOpen={narrow ? showQueue : showNowPlaying} onTogglePanel={togglePlayingView} playMode={playMode} onShuffle={() => void toggleShuffle()} onRepeat={() => void cycleRepeat()} onArtist={(track) => openArtist(nameKey(track.artist))} onAlbum={(track) => { const album = albums.find((item) => item.key === albumKey(track)); if (album) openAlbum(album) }} />

      {panels.map((id, layer) => <FloatingPanel key={id} id={id} layer={layer} title={panelTitles[id]} icon={panelIcons[id]} initial={panelStart[id]} min={panelMin[id]}
        onFocus={() => setPanels((open) => raisePanel(open, id))} onClose={() => setPanels((open) => open.filter((panel) => panel !== id))}>
        {id === 'equalizer' ? <EqualizerPanel onError={(error) => notify(displayError(error))} />
          : id === 'queue' ? <QueuePage tracks={playQueue} currentPath={playbackPath} onPlay={(track) => void playTrack(track, playQueue)} />
            : <LyricsView track={currentTrack} reducedMotion={!!reducedMotion} onSeek={seekLyrics} onError={(error) => notify(displayError(error))} />}
      </FloatingPanel>)}

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

// upNextShown is how many of the next songs the panel lists; the queue page has them all.
const upNextShown = 12

// NowPlayingPanel shows the current song large, with what plays next.
function NowPlayingPanel({ track, queue, onPlay, onFavorite, onClose, onShowQueue }: { onShowQueue: () => void; track?: Track; queue: Track[]; onPlay: (track: Track) => void; onFavorite?: () => void; onClose: () => void }) {
  const position = track ? queue.findIndex((item) => item.path === track.path) : -1
  const remaining = position >= 0 ? queue.length - position - 1 : 0
  const upNext = position >= 0 ? queue.slice(position + 1, position + 1 + upNextShown) : []
  return <aside className="now-panel" aria-label="Reproduciendo">
    {track?.coverUrl && <div className="now-panel-glow" style={{ backgroundImage: `url("${track.coverUrl}")` }} />}
    <div className="now-panel-head"><span>Reproduciendo</span><button className="icon-button" onClick={onClose} aria-label="Ocultar el panel Reproduciendo"><X size={16} /></button></div>
    {track ? <>
      <div className="now-panel-art" style={coverStyle(track, 0)}>{track.coverUrl ? null : <Disc3 size={64} strokeWidth={1} />}</div>
      <div className="now-panel-meta">
        <div><strong>{track.title}</strong><small>{track.artist}</small><small>{track.album}</small></div>
        {onFavorite && <button className={`player-heart ${track.favorite ? 'liked' : ''}`} onClick={onFavorite} aria-label={track.favorite ? 'Quitar de favoritos' : 'Añadir a favoritos'}><Heart size={18} fill={track.favorite ? 'currentColor' : 'none'} /></button>}
      </div>
      <div className="now-panel-label"><span>A continuación</span>{remaining > upNext.length && <button className="text-button" onClick={onShowQueue}>Ver cola completa ({remaining})</button>}</div>
      {upNext.length ? <div className="up-next">{upNext.map((item, index) => <button className="up-next-row" key={`${item.id}-${index}`} onClick={() => onPlay(item)}><span className="up-next-art" style={coverStyle(item, index)}>{item.coverUrl ? null : <Music2 size={13} />}</span><span><strong>{item.title}</strong><small>{item.artist}</small></span><span className="up-next-duration">{formatDuration(item.durationSeconds)}</span></button>)}</div> : <p className="now-panel-empty">No hay más canciones en la cola.</p>}
    </> : <div className="now-panel-idle"><Disc3 size={44} strokeWidth={1.1} /><p>Elige una canción y aquí verás su portada y lo que sigue.</p></div>}
  </aside>
}

function TrackPage({ title, tracks, currentPath, isPlaying, onPlay, onShuffle, onFavorite, onQueue, onPlaylist, onRefresh, onChooseFolders, scanning, showHeading = true, manageLibrary = false, defaultSort = 'title', toolbar = true, albumView = false }: { title: string; tracks: Track[]; currentPath: string; isPlaying: boolean; onPlay: (track: Track, list: Track[]) => void; onShuffle?: (list: Track[]) => void; onFavorite: (track: Track) => void; onQueue: (track: Track) => void; onPlaylist: (track: Track) => void; onRefresh: () => void; onChooseFolders: () => void; scanning: boolean; showHeading?: boolean; manageLibrary?: boolean; defaultSort?: SortKey; toolbar?: boolean; albumView?: boolean }) {
  const [sort, setSort] = useState<{ key: SortKey; descending: boolean }>({ key: defaultSort, descending: false })
  const sorted = useMemo(() => sortTracks(tracks, sort.key, sort.descending), [tracks, sort])
  const firstPlayable = sorted.find((track) => track.available)
  const totalSeconds = tracks.reduce((total, track) => total + (track.durationSeconds || 0), 0)
  const sortBy = (key: SortKey) => setSort((current) => (current.key === key ? { key, descending: !current.descending } : { key, descending: false }))
  const header = (key: SortKey, label: ReactNode, className = '') => <th className={className} aria-sort={sort.key === key ? (sort.descending ? 'descending' : 'ascending') : undefined}>
    <button className={`sort-button ${sort.key === key ? 'active' : ''}`} onClick={() => sortBy(key)} aria-label={`Ordenar por ${sortNames[key].toLowerCase()}`}>{label}{sort.key === key && (sort.descending ? <ChevronDown size={13} /> : <ChevronUp size={13} />)}</button>
  </th>
  const play = (track: Track) => track.available && onPlay(track, sorted)
  return <>
    {showHeading && <PageHeading title={title} subtitle={`${tracks.length} ${tracks.length === 1 ? 'canción' : 'canciones'} · ${formatLength(totalSeconds)}`} />}
    {toolbar && tracks.length > 0 && <div className="track-toolbar">
      <button className="toolbar-play" disabled={!firstPlayable} onClick={() => firstPlayable && onPlay(firstPlayable, sorted)} aria-label={`Reproducir ${title}`} title="Reproducir"><Play size={22} fill="currentColor" className="play-icon" /></button>
      {onShuffle && <button className="secondary-button" onClick={() => onShuffle(sorted)}><Shuffle size={15} /> Aleatorio</button>}
      <label className="sort-select"><span>Ordenar por</span>
        <select value={sort.key} onChange={(event) => setSort({ key: event.target.value as SortKey, descending: false })}>
          {(Object.keys(sortNames) as SortKey[]).filter((key) => key !== 'order' || defaultSort === 'order').map((key) => <option key={key} value={key}>{sortNames[key]}</option>)}
        </select>
      </label>
      {manageLibrary && <div className="toolbar-library">
        <button className="quiet-button compact-button" onClick={onRefresh} disabled={scanning}><RefreshCw className={scanning ? 'spinning' : ''} size={15} />{scanning ? 'Actualizando' : 'Actualizar'}</button>
        <button className="quiet-button compact-button" onClick={onChooseFolders}><FolderPlus size={15} /> Añadir carpeta</button>
      </div>}
    </div>}
    {tracks.length ? <div className="track-table-wrap" role="region" aria-label="Canciones; desplázate horizontalmente para ver todas las columnas" tabIndex={0}><table className="track-table"><thead><tr><th className="number-col">#</th>{header('title', 'Título')}{!albumView && header('album', 'Álbum')}{header('duration', <Clock3 size={14} />, 'duration-col')}<th /></tr></thead><tbody>{sorted.map((track, index) => {
      const current = currentPath === track.path
      return <tr key={track.id} className={`${current ? 'current' : ''} ${track.available ? '' : 'missing'}`} onDoubleClick={() => play(track)}>
        <td className="row-number"><button className="row-play" onClick={() => play(track)} aria-label={`Reproducir ${track.title}`}>{current ? <span className={`listening-bars ${isPlaying ? 'live' : ''}`} aria-hidden="true"><i /><i /><i /></span> : <span className="row-index">{String(albumView && track.trackNumber ? track.trackNumber : index + 1).padStart(2, '0')}</span>}<Play size={14} fill="currentColor" /></button></td>
        <td><button className="track-cell" onClick={() => play(track)}><span className="table-cover" style={coverStyle(track, index)}>{track.coverUrl ? null : <Music2 size={14} />}</span><span className="table-title"><strong>{track.title}{track.format && track.format !== 'MP3' && <span className="quality-tag">{track.format}</span>}</strong><small>{track.artist}{!track.available && <em> · Archivo no disponible</em>}</small></span></button></td>
        {!albumView && <td className="album-cell">{track.album}</td>}
        <td className="duration-cell">{formatDuration(track.durationSeconds)}</td>
        <td><div className="row-actions"><button className={`row-icon ${track.favorite ? 'liked' : ''}`} onClick={() => onFavorite(track)} aria-label={track.favorite ? 'Quitar de favoritos' : 'Añadir a favoritos'}><Heart size={15} fill={track.favorite ? 'currentColor' : 'none'} /></button><button className="row-icon" onClick={() => onQueue(track)} aria-label="Añadir a la cola"><Plus size={16} /></button><TrackMenu onPlaylist={() => onPlaylist(track)} onQueue={() => onQueue(track)} /></div></td>
      </tr>
    })}</tbody></table></div> : <div className="empty-state"><div className="empty-icon"><Music2 size={23} /></div><h2>No encontramos canciones.</h2><p>Prueba con otra búsqueda o conecta una carpeta con música.</p><button className="secondary-button" onClick={onChooseFolders}><FolderPlus size={15} /> Añadir carpeta</button></div>}
  </>
}

function TrackMenu({ onPlaylist, onQueue }: { onPlaylist: () => void; onQueue: () => void }) {
  const [open, setOpen] = useState(false)
  return <div className="track-menu"><button className="row-icon" onClick={() => setOpen((value) => !value)} aria-label="Más acciones"><Ellipsis size={17} /></button>{open && <><button className="menu-dismiss" onClick={() => setOpen(false)} aria-label="Cerrar menú" /><div className="track-menu-pop"><button onClick={() => { onQueue(); setOpen(false) }}><Plus size={14} /> Añadir a la cola</button><button onClick={() => { onPlaylist(); setOpen(false) }}><ListMusic size={14} /> Añadir a playlist</button></div></>}</div>
}

function PlaylistPage({ playlists, onOpen, onCreate }: { playlists: AppState['playlists']; onOpen: (id: string) => void; onCreate: () => void }) {
  return <><PageHeading title="Tus playlists" subtitle="Guarda tus canciones para volver a ellas cuando quieras." action={<button className="primary-button compact-button" onClick={onCreate}><Plus size={16} /> Nueva playlist</button>} />{playlists.length ? <div className="playlist-grid">{playlists.map((playlist, index) => <button className="playlist-card" key={playlist.id} onClick={() => onOpen(playlist.id)}><span className={`playlist-art playlist-art-${index % 4}`}><ListMusic size={27} strokeWidth={1.25} /></span><strong>{playlist.name}</strong><small>{playlist.trackCount} {playlist.trackCount === 1 ? 'canción' : 'canciones'}</small><span className="playlist-open"><ArrowUpRight size={15} /></span></button>)}</div> : <div className="empty-state"><div className="empty-icon"><ListMusic size={23} /></div><h2>Tu primera playlist empieza aquí.</h2><p>Junta canciones con una historia en común.</p><button className="secondary-button" onClick={onCreate}><Plus size={15} /> Crear playlist</button></div>}</>
}

function PlaylistDetail({ playlist, tracks, currentPath, isPlaying, onPlay, onFavorite, onQueue, onPlaylist, onRefresh }: { playlist?: AppState['playlists'][number]; tracks: Track[]; currentPath: string; isPlaying: boolean; onPlay: (track: Track, list?: Track[]) => void; onFavorite: (track: Track) => void; onQueue: (track: Track) => void; onPlaylist: (track: Track) => void; onRefresh: () => void }) {
  return <><PageHeading title={playlist?.name || 'Playlist'} subtitle={`${tracks.length} canciones · guardada en este equipo.`} />{tracks.length ? <TrackPage defaultSort="order" title={playlist?.name || 'Playlist'} tracks={tracks} currentPath={currentPath} isPlaying={isPlaying} onPlay={onPlay} onFavorite={onFavorite} onQueue={onQueue} onPlaylist={onPlaylist} onRefresh={onRefresh} onChooseFolders={() => {}} scanning={false} showHeading={false} /> : <div className="empty-state"><div className="empty-icon"><ListMusic size={23} /></div><h2>Esta playlist está lista para llenarse.</h2><p>Desde Canciones, abre el menú de acciones y elige “Añadir a playlist”.</p></div>}</>
}

function QueuePage({ tracks, currentPath, onPlay, onClose }: { tracks: Track[]; currentPath: string; onPlay: (track: Track) => void; onClose?: () => void }) {
  return <div className={onClose ? 'queue-panel' : ''}><div className="queue-heading"><h2>Cola de reproducción</h2>{onClose && <button className="icon-button" onClick={onClose} aria-label="Cerrar cola"><X size={17} /></button>}</div><p className="queue-caption">{tracks.length} canciones · solo para esta sesión</p>{tracks.length ? <div className="queue-list">{tracks.map((track, index) => <button className={`queue-row ${track.path === currentPath ? 'current' : ''}`} key={`${track.id}-${index}`} onClick={() => onPlay(track)}><span className="queue-index">{String(index + 1).padStart(2, '0')}</span><span className="queue-cover" style={coverStyle(track, index)}>{track.coverUrl ? null : <Music2 size={14} />}</span><span><strong>{track.title}</strong><small>{track.artist}</small></span><span className="queue-duration">{formatDuration(track.durationSeconds)}</span></button>)}</div> : <div className="queue-empty"><span className="empty-icon"><Shuffle size={21} /></span><p>Reproduce una canción para empezar a llenar tu cola.</p></div>}</div>
}

function SettingsPage({ state, activeTheme, editor, imagePreview, update, updateRunning, onUpdate, onTheme, onEdit, onSave, onImage, onAudio, onChooseFolders, onRemoveFolder, onRefresh, onEqualizer }: { state: AppState; activeTheme: Theme; editor: Theme; imagePreview: string; update: UpdateInfo; updateRunning: boolean; onUpdate: () => void; onTheme: (id: string) => void; onEdit: (theme: Theme) => void; onSave: () => void; onImage: (file?: File) => void; onAudio: (mode: string, deviceID: string) => void; onChooseFolders: () => void; onRemoveFolder: (path: string) => void; onRefresh: () => void; onEqualizer: () => void }) {
  const [name, setName] = useState(editor.name || 'Mi tema')
  const [lyricsOnline, setLyricsOnline] = useState(true)
  useEffect(() => { LibraryService.LyricsOnline().then(setLyricsOnline).catch(() => { /* keep the default */ }) }, [])
  const playback = usePlayback((current) => current)
  const outputMode = playback.outputMode || 'shared'
  const deviceID = playback.deviceID || ''
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
                  : 'Buscar actualizaciones'
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
  const [tab, setTab] = useState<SettingsTab>(() => (update.phase === 'available' ? 'about' : 'library'))
  const [showEditor, setShowEditor] = useState(false)
  const songs = state.tracks.length
  const head = (title: string, text: string, aside?: ReactNode) => <div className="settings-panel-head"><div><h2>{title}</h2><p>{text}</p></div>{aside}</div>
  return <>
    <PageHeading title="Ajustes" subtitle="Tu biblioteca, el sonido y el aspecto de la app." />
    <div className="settings-layout">
      <nav className="settings-nav" aria-label="Secciones de ajustes">{settingsTabs.map(({ id, label, icon: Icon }) => <button key={id} className={`settings-nav-item ${tab === id ? 'active' : ''}`} aria-current={tab === id ? 'page' : undefined} onClick={() => setTab(id)}>
        <Icon size={17} strokeWidth={1.8} /><span>{label}</span>{id === 'about' && update.phase === 'available' && <span className="update-dot" aria-label="Hay una versión nueva" />}
      </button>)}</nav>
      <div className="settings-panel">
        {tab === 'library' && <>
          {head('Biblioteca', 'Música Nativa lee tus carpetas sin mover, copiar ni cambiar tus archivos.', <button className="secondary-button compact-button" onClick={onRefresh} disabled={state.scanning}><RefreshCw size={14} className={state.scanning ? 'spinning' : ''} /> {state.scanning ? 'Actualizando…' : 'Volver a leer'}</button>)}
          <div className="folder-list">{state.folders.length ? state.folders.map((folder) => <div className="folder-row" key={folder}><span className="folder-mini-icon"><Library size={16} /></span><span><strong>{folder.split(/[\\/]/).pop() || folder}</strong><small>{folder}</small></span><button className="row-icon" onClick={() => onRemoveFolder(folder)} aria-label={`Quitar ${folder}`}><Trash2 size={15} /></button></div>) : <p className="folder-empty">Aún no has añadido carpetas.</p>}<button className="add-folder-row" onClick={onChooseFolders}><Plus size={15} /> Añadir carpeta</button></div>
          <p className="settings-note">{songs} {songs === 1 ? 'canción' : 'canciones'} en {state.folders.length} {state.folders.length === 1 ? 'carpeta' : 'carpetas'}. La biblioteca se actualiza sola cada vez que abres la app.</p>
        </>}

        {tab === 'sound' && <>
          {head('Sonido', 'Por dónde sale la música y cómo suena.', <span className={`output-status ${playback.outputReady ? 'ready' : ''}`}><i />{playback.outputReady ? 'Dispositivo listo' : 'Audio sin conectar'}</span>)}
          <div className="audio-settings">
            <div className="output-setting"><div><strong>Modo de salida</strong><small>Compartido funciona junto a otras aplicaciones. Exclusivo intenta reservar el dispositivo solo para la música.</small></div><div className="mode-switch"><button className={outputMode === 'shared' ? 'active' : ''} onClick={() => onAudio('shared', deviceID)}>Compartido</button><button className={outputMode === 'exclusive' ? 'active' : ''} onClick={() => onAudio('exclusive', deviceID)}>Exclusivo</button></div></div>
            <div className="output-setting device-setting"><div><strong>Dispositivo de salida</strong><small>Predeterminado sigue el dispositivo de audio elegido en Windows.</small></div><select aria-label="Dispositivo de salida" className="device-select" value={deviceID} onChange={(event) => onAudio(outputMode, event.target.value)}><option value="">Predeterminado de Windows</option>{state.outputDevices.map((device) => <option key={device.id} value={device.id}>{device.name}{device.default ? ' · predeterminado' : ''}</option>)}</select></div>
            <div className="output-setting eq-shortcut"><div><strong>Ecualizador</strong><small>10 bandas con preajustes, en vivo. También se abre desde la barra del reproductor o con la tecla E.</small></div><button className="secondary-button compact-button" onClick={onEqualizer}><SlidersVertical size={15} /> Abrir</button></div>
            {playback.error && <p className="audio-message"><span />{playback.error}</p>}
          </div>
        </>}

        {tab === 'look' && <>
          {head('Apariencia', 'Elige un tema para toda la app, o crea el tuyo.', <button className={`${showEditor ? 'secondary-button' : 'primary-button'} compact-button`} onClick={() => setShowEditor((open) => !open)}>{showEditor ? <><X size={14} /> Cerrar editor</> : <><Plus size={14} /> Crear un tema</>}</button>)}
          <div className="theme-grid">{state.themes.map((theme, index) => <div key={theme.id} className={`theme-card ${activeTheme.id === theme.id ? 'selected' : ''}`}><button className={`theme-preview theme-preview-${index % 4}`} style={{ background: theme.background }} onClick={() => onTheme(theme.id)} aria-label={`Usar el tema ${theme.name}`}><span className="theme-preview-window" style={{ background: theme.surface }}><i style={{ background: theme.accent }} /><i /><i /></span>{theme.backgroundImage && <span className="theme-photo" style={{ backgroundImage: `url("${theme.backgroundImage}")` }} />}{activeTheme.id === theme.id && <span className="theme-check"><Check size={14} /></span>}</button><div className="theme-card-meta"><div><strong>{theme.name}</strong><small>{theme.builtIn ? 'Incluido' : 'Creado por ti'}</small></div><button className={`apply-theme-button ${activeTheme.id === theme.id ? 'applied' : ''}`} onClick={() => onTheme(theme.id)}>{activeTheme.id === theme.id ? 'En uso' : 'Usar'}</button></div></div>)}</div>
          {showEditor && <div className="settings-subsection">
            <h3>Tu tema</h3>
            <div className="theme-editor"><div className="editor-preview" style={{ background: editor.background, backgroundImage: imagePreview || editor.backgroundImage ? `linear-gradient(180deg, rgba(0,0,0,.08), ${editor.background}d9), url("${imagePreview || editor.backgroundImage}")` : undefined }}><div className="editor-cover" style={coverStyle(undefined, 0)}><Disc3 size={29} /></div><div className="editor-playing"><strong>Una forma propia</strong><small>Tu música, a tu manera</small></div><div className="editor-controls"><span /><span /><span /></div></div><div className="editor-fields"><label className="field-label">Nombre del tema<input value={name} onChange={(event) => { setName(event.target.value); patchTheme('name', event.target.value) }} placeholder="Mi tema" maxLength={32} /></label><div className="color-fields"><ColorField label="Fondo" value={editor.background} onChange={(value) => patchTheme('background', value)} /><ColorField label="Acento" value={editor.accent} onChange={(value) => patchTheme('accent', value)} /><ColorField label="Superficie" value={editor.surface} onChange={(value) => patchTheme('surface', value)} /><ColorField label="Texto" value={editor.text} onChange={(value) => patchTheme('text', value)} /></div><label className="image-upload"><input type="file" accept="image/png,image/jpeg,image/webp" onChange={(event) => onImage(event.target.files?.[0])} /><span className="upload-icon"><FolderPlus size={16} /></span><span><strong>Usar imagen de fondo</strong><small>JPG, PNG o WebP · máximo 8 MB</small></span><ArrowUpRight size={15} /></label><button className="primary-button save-theme-button" onClick={onSave}><Check size={15} /> Guardar tema</button></div></div>
          </div>}
        </>}

        {tab === 'lyrics' && <>
          {head('Letras', 'Primero se buscan en tus archivos: un .lrc con el mismo nombre que la canción, o la letra guardada dentro de ella.')}
          <div className="audio-settings"><div className="output-setting lyrics-setting"><div><strong>Buscar letras en internet</strong><small>Si la canción no trae letra, se busca en LRCLIB, una base de datos libre. Para eso se envían el título, el artista, el álbum y la duración de la canción.</small></div><button role="switch" aria-checked={lyricsOnline} aria-label="Buscar letras en internet" className={`toggle-switch ${lyricsOnline ? 'on' : ''}`} onClick={() => { const next = !lyricsOnline; setLyricsOnline(next); void LibraryService.SetLyricsOnline(next) }}><span /></button></div></div>
          <p className="settings-note">Si una canción no tiene letra en ningún sitio, puedes añadirla tú desde la vista de letras (tecla L).</p>
        </>}

        {tab === 'keys' && <>
          {head('Atajos de teclado', 'Funcionan en toda la app mientras no estés escribiendo.')}
          <dl className="shortcut-list">{shortcutList.map(([keys, action]) => <div key={action}><dt>{keys.map((key) => <kbd key={key}>{key}</kbd>)}</dt><dd>{action}</dd></div>)}</dl>
        </>}

        {tab === 'about' && <>
          {head('Acerca de', `Música Nativa ${state.version}. Archivos tuyos, escucha tuya.`)}
          <div className="update-panel"><div className={`update-indicator ${update.phase}`}><span /></div><div className="update-copy"><strong>{update.name || 'Actualizaciones'}</strong><p role="status">{updateMessage[update.phase]}</p>{update.notes && (update.phase === 'available' || update.phase === 'ready' || update.phase === 'restarting') && <pre className="update-notes">{update.notes}</pre>}{update.phase === 'downloading' && <div className="update-progress" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={updatePercent}><span style={{ width: `${updatePercent}%` }} /></div>}</div><div className="update-actions"><button className={`${update.phase === 'available' ? 'primary-button' : 'secondary-button'} compact-button`} onClick={onUpdate} disabled={updateBusy}>{updateBusy ? <span className="update-spinner" /> : <RefreshCw size={14} />}{updateActionLabel}</button></div></div>
          <p className="settings-note">Al actualizar, la app descarga la versión nueva, comprueba que el archivo está bien y se reinicia sola.</p>
        </>}
      </div>
    </div>
  </>
}

function ColorField({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  const hex = /^#[0-9a-f]{6}$/i.test(value) ? value : '#11110f'
  return <label className="color-field"><span>{label}</span><span className="color-picker-wrap"><input type="color" value={hex} onChange={(event) => onChange(event.target.value)} /><i style={{ background: hex }} /></span><code>{value}</code></label>
}

function PlayerBar({ track, onToggle, onNext, onPrevious, onSeek, onVolume, onFavorite, onArtist, onAlbum, lyricsOpen, onLyrics, eqOpen, onEqualizer, panelOpen, onTogglePanel, playMode, onShuffle, onRepeat }: { track?: Track; onToggle: () => void; onNext: () => void; onPrevious: () => void; onSeek: (value: number) => Promise<unknown>; onVolume: (value: number) => void; onFavorite?: () => void; onArtist: (track: Track) => void; onAlbum: (track: Track) => void; lyricsOpen: boolean; onLyrics: () => void; eqOpen: boolean; onEqualizer: () => void; panelOpen: boolean; onTogglePanel: () => void; playMode: PlayMode; onShuffle: () => void; onRepeat: () => void }) {
  const repeatLabel = playMode.repeat === 'one' ? 'Repetir: esta canción' : playMode.repeat === 'all' ? 'Repetir: toda la cola' : 'Repetir: desactivado'
  const playback = usePlayback((current) => current)
  const tint = useCoverColor(track?.coverUrl)
  // The volume to go back to when the sound is turned on again.
  const unmuted = useRef(playback.volume > 0 ? playback.volume : 60)
  if (playback.volume > 0) unmuted.current = playback.volume
  const VolumeIcon = playback.volume === 0 ? VolumeX : playback.volume < 50 ? Volume1 : Volume2
  return <footer className={`player-bar ${playback.isPlaying ? 'is-playing' : ''}`} style={tint ? { '--tint': tint } as CSSProperties : undefined}>
    <span className="player-glow" aria-hidden="true" />
    <div className="now-playing">
      <button className="player-cover" style={coverStyle(track, 0)} onClick={onTogglePanel} aria-label="Ver lo que suena y lo que sigue" title="Reproduciendo y cola">{track?.coverUrl ? null : <Disc3 size={24} strokeWidth={1.2} />}</button>
      <span className="now-playing-text">
        <strong title={track?.title}>{track?.title || 'Elige lo que quieres escuchar'}</strong>
        {track ? <span className="now-playing-meta">
          <button className="meta-link" onClick={() => onArtist(track)} title={`Ver ${track.artist}`}>{track.artist}</button>
          <span aria-hidden="true">·</span>
          <button className="meta-link" onClick={() => onAlbum(track)} title={`Ver ${track.album}`}>{track.album}</button>
        </span> : <small>Pulsa reproducir y empieza tu música</small>}
      </span>
      {onFavorite && <button className={`player-heart ${track?.favorite ? 'liked' : ''}`} onClick={onFavorite} aria-label={track?.favorite ? 'Quitar de favoritos' : 'Añadir a favoritos'} title={track?.favorite ? 'Quitar de favoritos' : 'Añadir a favoritos'}><Heart size={17} fill={track?.favorite ? 'currentColor' : 'none'} /></button>}
    </div>
    <div className="player-center">
      <div className="transport-controls">
        <button className={`transport-subtle mode-button ${playMode.shuffle ? 'active' : ''}`} onClick={onShuffle} aria-pressed={playMode.shuffle} aria-label="Aleatorio" title="Aleatorio (S)"><Shuffle size={17} /></button>
        <button className="transport-subtle transport-skip" onClick={onPrevious} aria-label="Canción anterior" title="Anterior (Ctrl+←)"><SkipBack size={20} fill="currentColor" /></button>
        <button className="play-button" onClick={onToggle} aria-label={playback.isPlaying ? 'Pausar' : 'Reproducir'} title={`${playback.isPlaying ? 'Pausar' : 'Reproducir'} (Espacio)`}>{playback.isPlaying ? <Pause size={21} fill="currentColor" /> : <Play size={21} fill="currentColor" className="play-icon" />}</button>
        <button className="transport-subtle transport-skip" onClick={onNext} aria-label="Siguiente canción" title="Siguiente (Ctrl+→)"><SkipForward size={20} fill="currentColor" /></button>
        <button className={`transport-subtle mode-button ${playMode.repeat !== 'off' ? 'active' : ''}`} onClick={onRepeat} aria-label={repeatLabel} title={`${repeatLabel} (R)`}>{playMode.repeat === 'one' ? <Repeat1 size={17} /> : <Repeat size={17} />}</button>
      </div>
      <SeekBar positionMs={playback.positionMs} durationMs={playback.durationMs} disabled={!playback.path} onSeek={onSeek} />
    </div>
    <div className="player-tools">
      {!playback.outputReady && <span className="output-warning" title="No hay salida de audio: revisa el dispositivo en Ajustes, Sonido" role="status" aria-label="No hay salida de audio"><TriangleAlert size={16} /></span>}
      <div className="tool-group">
        <button className={`tool-button ${lyricsOpen ? 'active' : ''}`} onClick={onLyrics} aria-pressed={lyricsOpen} aria-label="Letra" title="Letra (L)"><MicVocal size={17} /></button>
        <button className={`tool-button ${panelOpen ? 'active' : ''}`} onClick={onTogglePanel} aria-pressed={panelOpen} aria-label="Reproduciendo y cola" title="Reproduciendo y cola"><PanelRight size={17} /></button>
        <button className={`tool-button mode-button eq-toggle ${playback.equalizerOn ? 'active' : ''}`} onClick={onEqualizer} aria-expanded={eqOpen} aria-label={playback.equalizerOn ? 'Ecualizador, activado' : 'Ecualizador'} title="Ecualizador (E)"><SlidersVertical size={17} /></button>
      </div>
      <div className="volume-control">
        <button className="tool-button" onClick={() => onVolume(playback.volume > 0 ? 0 : unmuted.current)} aria-label={playback.volume > 0 ? 'Silenciar' : 'Activar el sonido'} title={playback.volume > 0 ? 'Silenciar' : 'Activar el sonido'}><VolumeIcon size={18} /></button>
        <input aria-label="Volumen" className="player-range volume-range" type="range" min={0} max={100} value={playback.volume} onChange={(event) => onVolume(Number(event.target.value))} style={{ '--fill': `${playback.volume}%` } as CSSProperties} />
      </div>
    </div>
  </footer>
}

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
  const fill = durationMs > 0 ? Math.min(100, (shownMs / durationMs) * 100) : 0
  return <div className="seek-row"><span>{formatTime(shownMs)}</span><input ref={input} className="player-range" aria-label="Posición de la canción" aria-valuetext={`${formatTime(shownMs)} de ${formatTime(durationMs)}`} type="range" min={0} max={Math.max(1, durationMs)} value={shownMs} onChange={(event) => setDraggedMs(Number(event.target.value))} style={{ '--fill': `${fill}%` } as CSSProperties} disabled={disabled} /><span>{formatTime(durationMs)}</span></div>
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

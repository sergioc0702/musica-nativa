import { memo, useCallback, useEffect, useRef, useState } from 'react'
import { FileUp, MicVocal, Pencil, RefreshCw, Trash2 } from 'lucide-react'
import { LibraryService } from '../../bindings/musica-nativa'
import type { Lyrics } from '../../bindings/musica-nativa/models'
import type { Track } from '../../bindings/musica-nativa/internal/library/models'
import { positionNow } from '../lib/playback'
import { currentLineIndex, filledLetters } from '../lib/lyrics'

const sourceNames: Record<string, string> = {
  manual: 'Letra añadida por ti',
  lrc: 'Letra del archivo .lrc',
  tags: 'Letra guardada en la canción',
  lrclib: 'Letra de LRCLIB',
}

type Loaded = { trackID: string; lyrics?: Lyrics; loading: boolean }

// LyricsView shows the lyrics of the song that is playing. Synced lyrics
// light up letter by letter and follow the song; clicking a line jumps there.
export function LyricsView({ track, reducedMotion, onSeek, onError }: { track?: Track; reducedMotion: boolean; onSeek: (positionMs: number) => void; onError: (error: unknown) => void }) {
  const [loaded, setLoaded] = useState<Loaded>({ trackID: '', loading: false })
  const [draft, setDraft] = useState<string | null>(null)
  const trackID = track?.id ?? ''
  // The parent passes a new function on every render; asking again for
  // lyrics should depend on the song alone.
  const reportError = useRef(onError)
  reportError.current = onError

  const load = useCallback((request: (id: string) => Promise<Lyrics>) => {
    if (!trackID) return () => {}
    let stale = false
    setLoaded({ trackID, loading: true })
    request(trackID)
      .then((lyrics) => { if (!stale) setLoaded({ trackID, lyrics, loading: false }) })
      .catch((error) => { if (!stale) { setLoaded({ trackID, loading: false }); reportError.current(error) } })
    return () => { stale = true }
  }, [trackID])

  useEffect(() => {
    setDraft(null)
    return load((id) => LibraryService.Lyrics(id))
  }, [load])

  if (!track) {
    return <div className="lyrics-view"><div className="lyrics-empty"><MicVocal size={36} strokeWidth={1.3} /><p>Pon una canción para ver su letra aquí.</p></div></div>
  }
  const lyrics = loaded.trackID === trackID ? loaded.lyrics : undefined
  const loading = loaded.trackID !== trackID || loaded.loading

  const save = (text: string) => {
    LibraryService.SaveLyrics(trackID, text)
      .then((saved) => { setLoaded({ trackID, lyrics: saved, loading: false }); setDraft(null) })
      .catch(onError)
  }

  return <div className="lyrics-view">
    <header className="lyrics-head">
      <span className="lyrics-cover" style={track.coverUrl ? { backgroundImage: `url("${track.coverUrl}")` } : undefined} />
      <div>
        <h1>{track.title}</h1>
        <p>{track.artist}{lyrics?.found ? ` · ${sourceNames[lyrics.source] ?? 'Letra'}${lyrics.synced ? '' : ', sin tiempos'}` : ''}</p>
      </div>
      {draft === null && !loading && <div className="lyrics-actions">
        <button className="secondary-button compact-button" onClick={() => setDraft(lyrics?.source === 'manual' ? lyrics.text : '')}><Pencil size={14} /> {lyrics?.source === 'manual' ? 'Editar mi letra' : 'Añadir letra'}</button>
        {lyrics?.source !== 'manual' && lyrics?.source !== 'lrc' && lyrics?.source !== 'tags' && <button className="quiet-button compact-button" onClick={() => load((id) => LibraryService.SearchLyricsAgain(id))}><RefreshCw size={14} /> Buscar de nuevo</button>}
      </div>}
    </header>

    {draft !== null ? <LyricsEditor draft={draft} onDraft={setDraft} onSave={save} onCancel={() => setDraft(null)} canRemove={lyrics?.source === 'manual'} onError={onError} />
      : loading ? <p className="lyrics-note">Buscando la letra…</p>
        : !lyrics?.found ? <div className="lyrics-empty"><MicVocal size={36} strokeWidth={1.3} /><p>{lyrics?.message || 'No encontramos la letra de esta canción.'}</p><p className="lyrics-hint">Puedes pegarla tú con «Añadir letra», o poner un archivo .lrc con el mismo nombre junto a la canción.</p></div>
          : lyrics.instrumental && !lyrics.lines.length ? <div className="lyrics-empty"><MicVocal size={36} strokeWidth={1.3} /><p>Esta canción es instrumental.</p></div>
            : lyrics.synced ? <SyncedLyrics lyrics={lyrics} reducedMotion={reducedMotion} onSeek={onSeek} />
              : <div className="lyrics-plain"><p className="lyrics-note">Esta letra no tiene tiempos, así que no avanza con la canción.</p>{lyrics.lines.map((line, index) => line.text ? <p key={index}>{line.text}</p> : <div key={index} className="lyrics-gap" />)}</div>}
  </div>
}

function SyncedLyrics({ lyrics, reducedMotion, onSeek }: { lyrics: Lyrics; reducedMotion: boolean; onSeek: (positionMs: number) => void }) {
  const [position, setPosition] = useState(positionNow)
  const container = useRef<HTMLDivElement>(null)
  const scrolledByHand = useRef(0)

  useEffect(() => {
    let frame = 0
    const tick = () => {
      setPosition(positionNow())
      frame = requestAnimationFrame(tick)
    }
    frame = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frame)
  }, [])

  const lines = lyrics.lines
  const current = currentLineIndex(lines, position)
  const next = current + 1 < lines.length ? lines[current + 1].timeMs : Number.POSITIVE_INFINITY
  const filled = current >= 0 ? filledLetters(lines[current], next, position) : 0

  // Follow the song, unless the listener scrolled in the last few seconds.
  useEffect(() => {
    if (Date.now() - scrolledByHand.current < 4000) return
    container.current?.querySelector('.lyric-line.current')?.scrollIntoView({ block: 'center', behavior: reducedMotion ? 'auto' : 'smooth' })
  }, [current, reducedMotion])

  const handScroll = () => { scrolledByHand.current = Date.now() }
  return <div className="lyrics-lines" ref={container} onWheel={handScroll} onTouchMove={handScroll}>
    {lines.map((line, index) => <LyricRow key={index} text={line.text} timeMs={line.timeMs}
      state={index < current ? 'past' : index === current ? 'current' : 'future'}
      filled={index === current ? filled : 0} onSeek={onSeek} />)}
  </div>
}

// LyricRow only re-renders when its own state changes, so a frame redraws
// just the line being sung.
const LyricRow = memo(function LyricRow({ text, timeMs, state, filled, onSeek }: { text: string; timeMs: number; state: 'past' | 'current' | 'future'; filled: number; onSeek: (positionMs: number) => void }) {
  return <button className={`lyric-line ${state} ${text ? '' : 'pause'}`} onClick={() => onSeek(timeMs)} aria-current={state === 'current' ? 'true' : undefined}>
    {!text ? '♪' : state === 'current' ? <><span className="sung">{text.slice(0, filled)}</span><span className="unsung">{text.slice(filled)}</span></> : text}
  </button>
})

function LyricsEditor({ draft, onDraft, onSave, onCancel, canRemove, onError }: { draft: string; onDraft: (text: string) => void; onSave: (text: string) => void; onCancel: () => void; canRemove: boolean; onError: (error: unknown) => void }) {
  const loadFile = (file?: File) => {
    if (!file) return
    file.text().then(onDraft).catch(onError)
  }
  return <div className="lyrics-editor">
    <p className="lyrics-note">Pega la letra tal cual, o en formato LRC con tiempos como <code>[01:23.45]</code> para que avance con la canción.</p>
    <textarea value={draft} onChange={(event) => onDraft(event.target.value)} aria-label="Letra de la canción" placeholder={'[00:12.30]Primera línea\n[00:15.80]Segunda línea'} autoFocus />
    <div className="lyrics-editor-actions">
      <button className="primary-button compact-button" disabled={!draft.trim()} onClick={() => onSave(draft)}>Guardar</button>
      <button className="quiet-button compact-button" onClick={onCancel}>Cancelar</button>
      <label className="quiet-button compact-button lyrics-file"><FileUp size={14} /> Cargar .lrc<input type="file" accept=".lrc,.txt,text/plain" onChange={(event) => { loadFile(event.target.files?.[0]); event.target.value = '' }} /></label>
      {canRemove && <button className="quiet-button compact-button lyrics-remove" onClick={() => onSave('')}><Trash2 size={14} /> Quitar mi letra</button>}
    </div>
  </div>
}

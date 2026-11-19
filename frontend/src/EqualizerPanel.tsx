import { useEffect, useRef, useState } from 'react'
import { RotateCcw, X } from 'lucide-react'
import { LibraryService } from '../bindings/musica-nativa'
import { getPlayback, setPlayback } from './playback'
import { curvePath, eqBands, eqMaxGain, eqPresets, presetFor, snapGain } from './equalizer'

const curveWidth = 600
const curveHeight = 110
const curveRange = 15

type Settings = { enabled: boolean; bands: number[] }

const formatGain = (gain: number) => `${gain > 0 ? '+' : ''}${gain}`
const bandName = (frequency: number) => (frequency >= 1000 ? `${frequency / 1000} kHz` : `${Math.round(frequency)} Hz`)

// EqualizerPanel floats above the player bar. Each change is sent while the
// slider moves and the audio engine applies it within milliseconds; while one
// is on its way, only the newest waits behind it. Playing, pausing or seeking
// in the player bar keeps the panel open.
export function EqualizerPanel({ onClose, onError }: { onClose: () => void; onError: (error: unknown) => void }) {
  const [settings, setSettings] = useState<Settings>(() => {
    const playback = getPlayback()
    return { enabled: playback.equalizerOn, bands: eqBands.map((_, index) => snapGain(playback.equalizer[index] ?? 0)) }
  })
  const panel = useRef<HTMLDivElement>(null)
  const sending = useRef(false)
  const waiting = useRef<Settings | null>(null)
  const callbacks = useRef({ onClose, onError })
  callbacks.current = { onClose, onError }

  const send = (next: Settings) => {
    if (sending.current) {
      waiting.current = next
      return
    }
    sending.current = true
    LibraryService.SetEqualizer(next.enabled, next.bands)
      .then(setPlayback)
      .catch((error) => callbacks.current.onError(error))
      .finally(() => {
        sending.current = false
        const newest = waiting.current
        waiting.current = null
        if (newest) send(newest) // also after the panel closed: the last move is kept
      })
  }
  const apply = (next: Settings) => {
    setSettings(next)
    send(next)
  }
  const setBand = (index: number, value: number) =>
    apply({ ...settings, bands: settings.bands.map((gain, i) => (i === index ? snapGain(value) : gain)) })

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => { if (event.key === 'Escape') callbacks.current.onClose() }
    const onPointer = (event: PointerEvent) => {
      const target = event.target instanceof Element ? event.target : null
      if (target && !panel.current?.contains(target) && !target.closest('.player-bar')) callbacks.current.onClose()
    }
    window.addEventListener('keydown', onKey)
    window.addEventListener('pointerdown', onPointer)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('pointerdown', onPointer)
    }
  }, [])

  const preset = presetFor(settings.bands)
  const path = curvePath(settings.bands, curveWidth, curveHeight, curveRange)
  const middle = curveHeight / 2
  const gridY = (decibels: number) => middle - (decibels / curveRange) * middle

  return <div ref={panel} className={`eq-panel ${settings.enabled ? '' : 'off'}`} role="dialog" aria-label="Ecualizador">
    <div className="eq-panel-head">
      <div><strong>Ecualizador</strong><small>{settings.enabled ? preset?.name ?? 'Personalizado' : 'Apagado: suena el original'}</small></div>
      <button role="switch" aria-checked={settings.enabled} aria-label="Ecualizador activado" className={`toggle-switch ${settings.enabled ? 'on' : ''}`} onClick={() => apply({ ...settings, enabled: !settings.enabled })}><span /></button>
      <button className="icon-button" onClick={onClose} aria-label="Cerrar el ecualizador"><X size={16} /></button>
    </div>
    <div className="eq-presets">
      <label><span>Preajuste</span>
        <select value={preset?.id ?? 'custom'} onChange={(event) => {
          const chosen = eqPresets.find((item) => item.id === event.target.value)
          if (chosen) apply({ enabled: true, bands: [...chosen.bands] })
        }}>
          {eqPresets.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
          {!preset && <option value="custom" disabled>Personalizado</option>}
        </select>
      </label>
      <button className="quiet-button compact-button" disabled={preset?.id === 'flat'} onClick={() => apply({ ...settings, bands: eqBands.map(() => 0) })}><RotateCcw size={14} /> Restablecer</button>
    </div>
    <svg className="eq-curve" viewBox={`0 0 ${curveWidth} ${curveHeight}`} preserveAspectRatio="none" aria-hidden="true">
      {[12, 6, 0, -6, -12].map((decibels) => <line key={decibels} className={decibels === 0 ? 'zero' : ''} x1={0} x2={curveWidth} y1={gridY(decibels)} y2={gridY(decibels)} vectorEffect="non-scaling-stroke" />)}
      <path className="eq-curve-area" d={`${path} L${curveWidth} ${middle} L0 ${middle} Z`} />
      <path className="eq-curve-line" d={path} vectorEffect="non-scaling-stroke" />
    </svg>
    <div className="eq-sliders">
      {eqBands.map((band, index) => <label className="eq-slider" key={band.label} title="Doble clic: volver a 0 dB">
        <output>{formatGain(settings.bands[index])}</output>
        <input type="range" min={-eqMaxGain} max={eqMaxGain} step={0.5} value={settings.bands[index]} disabled={!settings.enabled}
          aria-label={bandName(band.frequency)} aria-valuetext={`${formatGain(settings.bands[index])} dB`}
          onChange={(event) => setBand(index, Number(event.target.value))} onDoubleClick={() => setBand(index, 0)} />
        <span>{band.label}</span>
      </label>)}
    </div>
  </div>
}

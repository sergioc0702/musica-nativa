import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent, type ReactNode } from 'react'
import { X } from 'lucide-react'
import { clampRect, type PanelID, type Rect, type Size } from './floating'

const storageKey = (id: PanelID) => `mn-panel-${id}`
const viewport = (): Size => ({ width: window.innerWidth, height: window.innerHeight })

function savedRect(id: PanelID): Rect | null {
  try {
    const raw = localStorage.getItem(storageKey(id))
    const rect = raw ? JSON.parse(raw) as Rect : null
    return rect && [rect.x, rect.y, rect.width, rect.height].every(Number.isFinite) ? rect : null
  } catch {
    return null
  }
}

function saveRect(id: PanelID, rect: Rect) {
  try { localStorage.setItem(storageKey(id), JSON.stringify(rect)) } catch { /* private window: forget it */ }
}

// FloatingPanel is a window inside the app: drag it by its bar, resize it
// from the corner, and it opens where it was left. Clicking it brings it to
// the front; Escape closes it while it has the focus.
export function FloatingPanel({ id, title, icon, initial, min, layer, onFocus, onClose, children }: { id: PanelID; title: string; icon: ReactNode; initial: (window: Size) => Rect; min: Size; layer: number; onFocus: () => void; onClose: () => void; children: ReactNode }) {
  const [rect, setRect] = useState<Rect>(() => clampRect(savedRect(id) ?? initial(viewport()), viewport(), min))
  const gesture = useRef<{ kind: 'move' | 'resize'; pointerX: number; pointerY: number; start: Rect } | null>(null)
  const latest = useRef(rect)
  latest.current = rect

  // A smaller window pulls the panel back inside.
  useEffect(() => {
    const onResize = () => setRect((current) => clampRect(current, viewport(), min))
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [min])

  const begin = (kind: 'move' | 'resize') => (event: ReactPointerEvent<HTMLElement>) => {
    if (event.button !== 0 || (kind === 'move' && (event.target as Element).closest('button'))) return
    event.preventDefault()
    try { event.currentTarget.setPointerCapture(event.pointerId) } catch { /* the pointer is already gone */ }
    gesture.current = { kind, pointerX: event.clientX, pointerY: event.clientY, start: latest.current }
  }
  const move = (event: ReactPointerEvent<HTMLElement>) => {
    const active = gesture.current
    if (!active) return
    const dx = event.clientX - active.pointerX
    const dy = event.clientY - active.pointerY
    const next = active.kind === 'move'
      ? { ...active.start, x: active.start.x + dx, y: active.start.y + dy }
      : { ...active.start, width: active.start.width + dx, height: active.start.height + dy }
    // Kept in the ref right away: the pointer can be released before React
    // renders this position, and that is the one to remember.
    latest.current = clampRect(next, viewport(), min)
    setRect(latest.current)
  }
  const end = () => {
    if (!gesture.current) return
    gesture.current = null
    saveRect(id, latest.current)
  }

  return <section className="floating-panel" role="dialog" aria-label={title}
    style={{ left: rect.x, top: rect.y, width: rect.width, height: rect.height, zIndex: 30 + layer }}
    onPointerDownCapture={onFocus}
    onKeyDown={(event) => { if (event.key === 'Escape') { event.stopPropagation(); onClose() } }}>
    <header className="floating-head" onPointerDown={begin('move')} onPointerMove={move} onPointerUp={end} onPointerCancel={end}>
      <span className="floating-icon">{icon}</span>
      <strong>{title}</strong>
      <button className="icon-button" onClick={onClose} aria-label={`Cerrar ${title.toLowerCase()}`}><X size={16} /></button>
    </header>
    <div className="floating-body">{children}</div>
    <span className="floating-resize" onPointerDown={begin('resize')} onPointerMove={move} onPointerUp={end} onPointerCancel={end} aria-hidden="true" />
  </section>
}

// Placement of the floating panels (equalizer, queue, lyrics).

export type PanelID = 'equalizer' | 'queue' | 'lyrics'
export type Rect = { x: number; y: number; width: number; height: number }
export type Size = { width: number; height: number }

// How much of a panel must stay in the window: enough of its bar to grab it.
const keepVisible = 120
const barHeight = 44

// clampRect keeps a panel within its minimum size and the window's, with its
// title bar inside the window so it can always be dragged back.
export function clampRect(rect: Rect, window: Size, min: Size): Rect {
  const width = Math.round(Math.min(Math.max(rect.width, min.width), window.width))
  const height = Math.round(Math.min(Math.max(rect.height, min.height), window.height))
  const x = Math.round(Math.min(Math.max(rect.x, -(width - keepVisible)), window.width - keepVisible))
  const y = Math.round(Math.min(Math.max(rect.y, 0), window.height - barHeight))
  return { x, y, width, height }
}

// togglePanel opens a panel on top of the others, or closes it if open.
// The list goes from the panel at the back to the one in front.
export function togglePanel(open: PanelID[], id: PanelID): PanelID[] {
  return open.includes(id) ? open.filter((panel) => panel !== id) : [...open, id]
}

// raisePanel brings an open panel to the front.
export function raisePanel(open: PanelID[], id: PanelID): PanelID[] {
  if (!open.includes(id) || open[open.length - 1] === id) return open
  return [...open.filter((panel) => panel !== id), id]
}

import type { CSSProperties } from 'react'
import type { Track } from '../bindings/musica-nativa/internal/library/models'

export function formatTime(milliseconds: number) {
  const seconds = Math.max(0, Math.floor(milliseconds / 1000))
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
}

export function formatDuration(seconds: number) {
  return seconds > 0 ? formatTime(seconds * 1000) : '—:—'
}

const fallbackCovers = [
  'linear-gradient(145deg, #b77f5c, #473831 54%, #161817)',
  'linear-gradient(145deg, #8f9c80, #40483e 54%, #161817)',
  'linear-gradient(145deg, #7f8fa2, #39414a 54%, #17191b)',
  'linear-gradient(145deg, #a48d77, #51443a 54%, #171716)',
  'linear-gradient(145deg, #8c7384, #443944 54%, #191719)',
]

// coverStyle shows a song's cover, or a soft gradient when it has none.
export function coverStyle(track?: Track, index = 0): CSSProperties {
  if (track?.coverUrl) return { backgroundImage: `url("${track.coverUrl}")` }
  return { backgroundImage: fallbackCovers[index % fallbackCovers.length] }
}

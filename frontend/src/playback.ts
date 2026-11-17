import { useSyncExternalStore } from 'react'
import type { PlaybackState } from '../bindings/musica-nativa/models'

// The backend reports playback four times per second while a song plays.
// Keeping that state outside React state lets only the components that show
// it re-render, instead of the whole library view.
let current: PlaybackState = {
  path: '', isPlaying: false, positionMs: 0, durationMs: 0, volume: 72, outputMode: 'shared',
  deviceID: '', outputReady: false, equalizerOn: false, equalizer: [0, 0, 0],
}
const listeners = new Set<() => void>()

export function setPlayback(next: PlaybackState) {
  current = next
  listeners.forEach((listener) => listener())
}

export function getPlayback() {
  return current
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}

// usePlayback re-renders the caller only when the selected value changes.
// Select a primitive or the whole state: a selector that builds a new object
// would re-render on every update.
export function usePlayback<T>(select: (playback: PlaybackState) => T): T {
  return useSyncExternalStore(subscribe, () => select(current))
}

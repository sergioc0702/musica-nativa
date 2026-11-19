// Timing of synced lyrics for the karaoke view.

export type TimedWord = { timeMs: number; text: string }
export type TimedLine = { timeMs: number; text: string; words?: TimedWord[] | null }

// Without word timings, a line is assumed to be sung at about nine letters a
// second plus a moment, never slower than the time until the next line.
const letterMs = 110
const lineExtraMs = 600
const shortestLineMs = 1200
const shortestWordMs = 400

// currentLineIndex is the line being sung at positionMs: the last one that
// has started, or -1 before the first.
export function currentLineIndex(lines: TimedLine[], positionMs: number): number {
  let index = -1
  for (let i = 0; i < lines.length && lines[i].timeMs <= positionMs; i++) index = i
  return index
}

function singingMs(text: string, available: number): number {
  return Math.max(1, Math.min(available, Math.max(shortestLineMs, text.length * letterMs + lineExtraMs)))
}

// filledLetters is how many letters of line have been sung at positionMs.
// nextTimeMs is when the following line starts (Infinity for the last one).
export function filledLetters(line: TimedLine, nextTimeMs: number, positionMs: number): number {
  const length = line.text.length
  if (positionMs < line.timeMs || length === 0) return 0
  const words = line.words ?? []
  if (!words.length) {
    return Math.min(length, Math.round(((positionMs - line.timeMs) / singingMs(line.text, nextTimeMs - line.timeMs)) * length))
  }
  // Any text before the first timed word counts as sung with it.
  let filled = Math.max(0, length - words.reduce((total, word) => total + word.text.length, 0))
  for (let i = 0; i < words.length; i++) {
    const word = words[i]
    const end = i + 1 < words.length
      ? words[i + 1].timeMs
      : Math.min(nextTimeMs, word.timeMs + Math.max(shortestWordMs, word.text.length * letterMs))
    if (positionMs < word.timeMs) break
    if (positionMs >= end) {
      filled += word.text.length
      continue
    }
    filled += Math.round(((positionMs - word.timeMs) / Math.max(1, end - word.timeMs)) * word.text.length)
    break
  }
  return Math.min(filled, length)
}

import { useEffect, useState } from 'react'

// vividColor picks the color a cover is known by from its RGBA pixels: the
// saturated parts weigh most, near-black, near-white and gray are left out.
// The result is kept light and colorful enough to tint a dark page, or null
// when the cover has no color at all.
export function vividColor(data: Uint8ClampedArray): string | null {
  let red = 0, green = 0, blue = 0, total = 0
  for (let i = 0; i + 3 < data.length; i += 4) {
    if (data[i + 3] < 128) continue
    const r = data[i], g = data[i + 1], b = data[i + 2]
    const high = Math.max(r, g, b), low = Math.min(r, g, b)
    const chroma = (high - low) / 255
    const lightness = (high + low) / 510
    if (chroma < 0.12 || lightness < 0.1 || lightness > 0.94) continue
    const weight = chroma * chroma
    red += r * weight
    green += g * weight
    blue += b * weight
    total += weight
  }
  if (total === 0) return null
  const [h, s, l] = toHSL(red / total, green / total, blue / total)
  const [r, g, b] = fromHSL(h, Math.min(0.85, Math.max(0.4, s)), Math.min(0.62, Math.max(0.42, l)))
  return `rgb(${Math.round(r)} ${Math.round(g)} ${Math.round(b)})`
}

function toHSL(r: number, g: number, b: number): [number, number, number] {
  r /= 255; g /= 255; b /= 255
  const high = Math.max(r, g, b), low = Math.min(r, g, b)
  const l = (high + low) / 2
  if (high === low) return [0, 0, l]
  const d = high - low
  const s = l > 0.5 ? d / (2 - high - low) : d / (high + low)
  const h = high === r ? (g - b) / d + (g < b ? 6 : 0) : high === g ? (b - r) / d + 2 : (r - g) / d + 4
  return [h / 6, s, l]
}

function fromHSL(h: number, s: number, l: number): [number, number, number] {
  const q = l < 0.5 ? l * (1 + s) : l + s - l * s
  const p = 2 * l - q
  const channel = (t: number) => {
    t = (t + 1) % 1
    if (t < 1 / 6) return p + (q - p) * 6 * t
    if (t < 1 / 2) return q
    if (t < 2 / 3) return p + (q - p) * (2 / 3 - t) * 6
    return p
  }
  return [channel(h + 1 / 3) * 255, channel(h) * 255, channel(h - 1 / 3) * 255]
}

const known = new Map<string, string | null>()

// useCoverColor reads a cover (served by the app itself, so the canvas may
// read it) and returns its color, remembered per cover.
export function useCoverColor(url?: string): string | null {
  const [color, setColor] = useState<string | null>(() => (url ? known.get(url) ?? null : null))
  useEffect(() => {
    if (!url) { setColor(null); return }
    if (known.has(url)) { setColor(known.get(url) ?? null); return }
    let stale = false
    const image = new Image()
    image.onload = () => {
      const canvas = document.createElement('canvas')
      canvas.width = canvas.height = 24
      const context = canvas.getContext('2d', { willReadFrequently: true })
      if (!context) return
      context.drawImage(image, 0, 0, 24, 24)
      const found = vividColor(context.getImageData(0, 0, 24, 24).data)
      known.set(url, found)
      if (!stale) setColor(found)
    }
    image.src = url
    return () => { stale = true }
  }, [url])
  return color
}

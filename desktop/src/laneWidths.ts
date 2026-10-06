// Lane widths the user set by dragging a lane's edge, so long branch names can be read.
// Stored per stack, keyed by its bottom branch: stack ids are not stable across sessions.

const storageKey = "buti.laneWidths"
export const defaultLaneWidth = 256
export const minLaneWidth = 192
export const maxLaneWidth = 720

export type LaneWidths = Record<string, number>

export function clampLaneWidth(w: number): number {
  return Math.round(Math.min(Math.max(w, minLaneWidth), maxLaneWidth))
}

/** setLaneWidth returns widths with key set to w, or removed when w is the default. */
export function setLaneWidth(widths: LaneWidths, key: string, w: number): LaneWidths {
  const { [key]: _, ...rest } = widths
  const c = clampLaneWidth(w)
  return c === defaultLaneWidth ? rest : { ...rest, [key]: c }
}

/** parseLaneWidths reads the stored map, dropping anything that is not a width. */
export function parseLaneWidths(raw: string | null): LaneWidths {
  if (!raw) return {}
  try {
    const v: unknown = JSON.parse(raw)
    if (!v || typeof v !== "object" || Array.isArray(v)) return {}
    const out: LaneWidths = {}
    for (const [k, w] of Object.entries(v)) {
      if (typeof w === "number" && Number.isFinite(w)) out[k] = clampLaneWidth(w)
    }
    return out
  } catch {
    return {}
  }
}

export function loadLaneWidths(): LaneWidths {
  try {
    return parseLaneWidths(localStorage.getItem(storageKey))
  } catch {
    return {}
  }
}

export function saveLaneWidths(widths: LaneWidths) {
  try {
    localStorage.setItem(storageKey, JSON.stringify(widths))
  } catch {
    // Storage is unavailable or full; widths are a convenience.
  }
}

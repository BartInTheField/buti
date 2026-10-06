// Recently opened repositories, newest first. The desktop app is not started in a folder the way
// the TUI is, so the folder picker offers these, and a window started outside a repository reopens one.

const storageKey = "buti.recentRepos"
export const maxRecent = 10

/** addRecent puts dir first, without duplicates, keeping at most max. */
export function addRecent(list: string[], dir: string, max = maxRecent): string[] {
  return [dir, ...list.filter((d) => d !== dir)].slice(0, max)
}

export function removeRecent(list: string[], dir: string): string[] {
  return list.filter((d) => d !== dir)
}

/** repoName is the last path segment, the way the header names a repository. */
export function repoName(dir: string): string {
  return dir.split(/[/\\]/).filter(Boolean).pop() ?? dir
}

/** parseRecent reads the stored list, dropping anything that is not a list of paths. */
export function parseRecent(raw: string | null): string[] {
  if (!raw) return []
  try {
    const v: unknown = JSON.parse(raw)
    return Array.isArray(v) ? v.filter((d): d is string => typeof d === "string" && d !== "").slice(0, maxRecent) : []
  } catch {
    return []
  }
}

export function loadRecent(): string[] {
  try {
    return parseRecent(localStorage.getItem(storageKey))
  } catch {
    return []
  }
}

function save(list: string[]) {
  try {
    localStorage.setItem(storageKey, JSON.stringify(list))
  } catch {
    // Storage is unavailable or full; the list is a convenience.
  }
}

export function rememberRepo(dir: string) {
  save(addRecent(loadRecent(), dir))
}

export function forgetRepo(dir: string) {
  save(removeRecent(loadRecent(), dir))
}

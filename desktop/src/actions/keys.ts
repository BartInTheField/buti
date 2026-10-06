// Key strings use the TUI's notation (internal/ui/actions.go): "c", "R", "space",
// "ctrl+r", plus "mod+k" for cmd on macOS / ctrl elsewhere.

export const isMac =
  typeof navigator !== "undefined" && /mac|iphone|ipad/i.test(navigator.platform)

const named: Record<string, string> = {
  " ": "space",
  Enter: "enter",
  Escape: "escape",
  Backspace: "backspace",
  Delete: "delete",
  Tab: "tab",
  ArrowUp: "up",
  ArrowDown: "down",
  ArrowLeft: "left",
  ArrowRight: "right",
}

/**
 * eventKeys returns the key strings an event matches: the literal form ("ctrl+r",
 * "meta+k") and the "mod+" alias. Shift is folded into the character for printable
 * keys ("R", "?"), so it only appears for named keys ("shift+tab").
 */
export function eventKeys(e: KeyboardEvent): string[] {
  let key = named[e.key] ?? e.key
  if (key.length === 1 && /[a-z]/i.test(key) && (e.ctrlKey || e.metaKey || e.altKey)) {
    key = key.toLowerCase() // ctrl+shift+r would otherwise be "ctrl+R"
    if (e.shiftKey) key = key.toUpperCase()
  }
  const printable = key.length === 1
  const mods = (m: { ctrl: boolean; meta: boolean }) =>
    [
      m.ctrl && "ctrl",
      m.meta && "meta",
      e.altKey && "alt",
      e.shiftKey && !printable && "shift",
    ].filter(Boolean)
  const literal = [...mods({ ctrl: e.ctrlKey, meta: e.metaKey }), key].join("+")
  const modDown = isMac ? e.metaKey : e.ctrlKey
  if (!modDown) return [literal]
  const alias = [
    "mod",
    ...mods({ ctrl: isMac && e.ctrlKey, meta: !isMac && e.metaKey }),
    key,
  ].join("+")
  return [literal, alias]
}

const glyphs: Record<string, string> = isMac
  ? { mod: "⌘", meta: "⌘", ctrl: "⌃", alt: "⌥", shift: "⇧" }
  : { mod: "Ctrl", meta: "Win", ctrl: "Ctrl", alt: "Alt", shift: "Shift" }

/** formatKey renders a key string for a Kbd: "mod+k" → "⌘K" / "Ctrl+K". */
export function formatKey(key: string): string {
  const parts = key.split("+").filter(Boolean)
  if (key.endsWith("++")) parts.push("+")
  const out = parts.map((p, i) => {
    if (i < parts.length - 1) return glyphs[p] ?? p
    if (p === "space") return "Space"
    if (p === "enter") return "↵"
    if (p === "escape") return "Esc"
    return p.length === 1 && parts.length > 1 ? p.toUpperCase() : p
  })
  return out.join(isMac ? "" : "+")
}

/** typingIn reports whether the event comes from a field or an open overlay that owns the keys. */
export function typingIn(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  if (target.isContentEditable) return true
  if (target.closest("input, textarea, select")) return true
  return Boolean(
    target.closest('[role="dialog"], [role="alertdialog"], [role="menu"], [role="listbox"]'),
  )
}

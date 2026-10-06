/** copyText puts text on the clipboard, falling back to a hidden textarea where the API is missing. */
export async function copyText(text: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text)
    return
  } catch {
    // Not a secure context or no permission: try the old way.
  }
  // Selecting the textarea takes focus; give it back so the diff pane keeps its cursor.
  const had = document.activeElement instanceof HTMLElement ? document.activeElement : null
  const ta = document.createElement("textarea")
  ta.value = text
  ta.setAttribute("readonly", "")
  ta.style.position = "fixed"
  ta.style.opacity = "0"
  document.body.appendChild(ta)
  ta.select()
  const ok = document.execCommand("copy")
  ta.remove()
  had?.focus({ preventScroll: true })
  if (!ok) throw new Error("Could not copy to the clipboard")
}

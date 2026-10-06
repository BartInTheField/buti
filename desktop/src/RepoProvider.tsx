import { useState, type ReactNode } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { openRepo, type ApiConfig } from "./api"
import { useDialogs } from "./dialogs/dialogs"
import { cancelTarget } from "./components/target/store"
import { forgetRepo, loadRecent, rememberRepo } from "./recent"
import { RepoContext, useRepo, type RepoSwitcher } from "./repo"

/** pickFolder is the OS folder dialog inside Tauri; null when cancelled, undefined outside Tauri. */
async function pickFolder(defaultPath?: string): Promise<string | null | undefined> {
  const { isTauri } = await import("@tauri-apps/api/core")
  if (!isTauri()) return undefined
  const { open } = await import("@tauri-apps/plugin-dialog")
  const picked = await open({ directory: true, multiple: false, defaultPath, title: "Open a GitButler repository" })
  return typeof picked === "string" ? picked : null
}

export function RepoProvider({ cfg, children }: { cfg: ApiConfig; children: ReactNode }) {
  const qc = useQueryClient()
  const dialogs = useDialogs()
  const repo = useRepo(cfg)
  const [recent, setRecent] = useState(loadRecent)
  const [switching, setSwitching] = useState(false)

  async function open(dir: string) {
    setSwitching(true)
    try {
      await openRepo(cfg, dir)
      cancelTarget()
      // Everything cached belongs to the old repository; resetting shows "Reading workspace…"
      // rather than the old lanes, and the workspace screen mounts afresh for the new one.
      await qc.resetQueries({ predicate: (q) => q.queryKey[0] !== "api-config" })
    } finally {
      setSwitching(false)
    }
  }

  async function choose() {
    const current = repo.data?.dir
    let dir = await pickFolder(current)
    if (dir === undefined) {
      // A browser (Vite against `buti desktop --serve`) has no folder dialog.
      dir = await dialogs.prompt({
        title: "Open repository",
        description: "The absolute path of a GitButler repository.",
        initial: current,
        placeholder: "/path/to/repository",
        submitLabel: "Open",
        validate: (v) => (v.trim() === "" ? "Enter a path" : null),
      })
    }
    if (!dir) return false
    await open(dir.trim())
    return true
  }

  const value: RepoSwitcher = {
    dir: repo.data?.dir,
    recent,
    switching,
    open,
    choose,
    shown: (dir) => {
      if (recent[0] === dir) return
      rememberRepo(dir)
      setRecent(loadRecent())
    },
    forget: (dir) => {
      forgetRepo(dir)
      setRecent(loadRecent())
    },
  }
  return <RepoContext.Provider value={value}>{children}</RepoContext.Provider>
}

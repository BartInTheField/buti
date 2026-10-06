import { createContext, useContext } from "react"
import { useQuery } from "@tanstack/react-query"
import { fetchRepo, type ApiConfig } from "./api"

// The desktop app is not started in a repository the way the TUI is: the folder picker
// points the running API at one (POST /repo) and the window remembers what it opened.

export type RepoSwitcher = {
  /** The repository the API serves, once known. */
  dir: string | undefined
  /** Recently opened repositories, newest first, the current one included. */
  recent: string[]
  /** A switch is in flight. */
  switching: boolean
  /** open switches to dir; it rejects, and the current repository stays, when dir is not a GitButler workspace. */
  open: (dir: string) => Promise<void>
  /** choose asks for a folder (the native dialog, or a path in a browser) and opens it. False when cancelled. */
  choose: () => Promise<boolean>
  /** shown records a repository whose workspace loaded, so the picker and the next start offer it. */
  shown: (dir: string) => void
  forget: (dir: string) => void
}

export const RepoContext = createContext<RepoSwitcher | null>(null)

export function useRepoSwitcher(): RepoSwitcher {
  const r = useContext(RepoContext)
  if (!r) throw new Error("useRepoSwitcher outside RepoProvider")
  return r
}

export const repoKey = (url: string) => ["repo", url] as const

export function useRepo(cfg: ApiConfig | undefined) {
  return useQuery({
    queryKey: repoKey(cfg?.url ?? ""),
    queryFn: () => fetchRepo(cfg!),
    enabled: Boolean(cfg?.url),
    retry: false,
  })
}

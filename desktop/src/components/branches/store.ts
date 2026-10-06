import { useSyncExternalStore } from "react"

// The operation history sheet and the pull request dialog are opened by actions, which
// run outside React. This small store lets an action open them and await the result;
// BranchesLayer renders whatever is open.

export type PullRequestDraft = { message: string; draft: boolean }

type State = {
  oplog: boolean
  pr: { branch: string; resolve: (v: PullRequestDraft | null) => void } | null
}

let state: State = { oplog: false, pr: null }
const listeners = new Set<() => void>()

function set(next: Partial<State>) {
  state = { ...state, ...next }
  listeners.forEach((l) => l())
}

export function useBranchesLayer(): State {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => state,
  )
}

export function setOplogOpen(open: boolean) {
  set({ oplog: open })
}

/** askPullRequest opens the pull request dialog; null when it is cancelled. */
export function askPullRequest(branch: string): Promise<PullRequestDraft | null> {
  state.pr?.resolve(null)
  return new Promise((resolve) => set({ pr: { branch, resolve } }))
}

export function settlePullRequest(value: PullRequestDraft | null) {
  const pr = state.pr
  set({ pr: null })
  pr?.resolve(value)
}

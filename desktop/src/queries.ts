import { useSyncExternalStore } from "react"
import {
  keepPreviousData,
  useIsMutating,
  useMutation,
  useMutationState,
  useQuery,
  useQueryClient,
  type QueryClient,
} from "@tanstack/react-query"
import {
  fetchBranches,
  fetchDiff,
  fetchOplog,
  fetchSyncedWorkspace,
  fetchWorkspace,
  loadConfig,
  ops,
  type ApiConfig,
  type OpName,
  type OpResult,
} from "./api"

// The CLI and repository errors do not change on a retry. Refresh refetches.
const noRetry = { retry: false }

const workspaceRefetchMs = 5_000

// Background refetches pause while something holds them (a drag in progress), so
// droppables never re-render or move under the pointer mid-gesture.
let refetchHolds = 0
const refetchListeners = new Set<() => void>()

/** holdRefetch pauses the workspace poll until the returned release is called. */
export function holdRefetch(): () => void {
  refetchHolds++
  refetchListeners.forEach((l) => l())
  let released = false
  return () => {
    if (released) return
    released = true
    refetchHolds--
    refetchListeners.forEach((l) => l())
  }
}

function useRefetchHeld(): boolean {
  return useSyncExternalStore(
    (l) => {
      refetchListeners.add(l)
      return () => refetchListeners.delete(l)
    },
    () => refetchHolds > 0,
  )
}

export function useApiConfig() {
  return useQuery({
    queryKey: ["api-config"],
    queryFn: loadConfig,
    staleTime: Infinity,
    ...noRetry,
  })
}

export const workspaceKey = (url: string | undefined) => ["workspace", url] as const

export function useWorkspace(url: string | undefined, token: string | undefined) {
  const held = useRefetchHeld()
  return useQuery({
    queryKey: workspaceKey(url),
    queryFn: () => fetchWorkspace({ url: url ?? "", token: token ?? "" }),
    enabled: Boolean(url),
    refetchInterval: held ? false : workspaceRefetchMs,
    refetchOnWindowFocus: !held,
    // Never blank the screen between fetches; structural sharing (the default) keeps
    // unchanged stacks referentially equal so rows do not re-render.
    placeholderData: keepPreviousData,
    ...noRetry,
  })
}

export function useDiff(
  url: string | undefined,
  token: string | undefined,
  id: string | null,
) {
  return useQuery({
    queryKey: ["diff", url, id],
    queryFn: () => fetchDiff({ url: url ?? "", token: token ?? "" }, id ?? ""),
    enabled: Boolean(url) && id !== null,
    placeholderData: keepPreviousData,
    ...noRetry,
  })
}

export function useOplog(cfg: ApiConfig, enabled = true) {
  return useQuery({
    queryKey: ["oplog", cfg.url],
    queryFn: () => fetchOplog(cfg),
    enabled,
    ...noRetry,
  })
}

export function useBranches(cfg: ApiConfig, enabled = true) {
  return useQuery({
    queryKey: ["branches", cfg.url],
    queryFn: () => fetchBranches(cfg),
    enabled,
    ...noRetry,
  })
}

/** invalidateAfterOp refetches everything a `but` mutation can change. */
export async function invalidateAfterOp(qc: QueryClient, url: string) {
  await Promise.all(
    ["workspace", "diff", "oplog", "branches"].map((k) =>
      qc.invalidateQueries({ queryKey: [k, url] }),
    ),
  )
}

type OpArgs<K extends OpName> = Parameters<(typeof ops)[K]>[1]

/** RunOp calls one `POST /ops/...` endpoint and resolves after the workspace refetched. */
export type RunOp = <K extends OpName>(
  name: K,
  ...args: OpArgs<K> extends undefined ? [] : [OpArgs<K>]
) => Promise<OpResult>

export type WorkspaceOps = {
  cfg: ApiConfig
  /** Any mutation in flight; drops and actions wait for it, like the TUI's busy flag. */
  busy: boolean
  run: RunOp
  /** refresh refetches the workspace; sync also syncs pull requests from the forge. */
  refresh: (opts?: { sync?: boolean }) => Promise<void>
  invalidate: () => Promise<void>
}

const opKey = (url: string) => ["op", url] as const

export function useWorkspaceOps(url: string, token: string): WorkspaceOps {
  const qc = useQueryClient()
  const cfg: ApiConfig = { url, token }
  const busy = useIsMutating({ mutationKey: opKey(url) }) > 0

  const invalidate = () => invalidateAfterOp(qc, url)

  const mutation = useMutation({
    mutationKey: opKey(url),
    mutationFn: ({ name, args }: { name: OpName; args: unknown }) =>
      (ops[name] as (c: ApiConfig, a: unknown) => Promise<OpResult>)(cfg, args),
    // Refetch even after a failure: a partly applied op (absorb of several sources) still changed things.
    onSettled: invalidate,
  })

  const run: RunOp = (name, ...args) => mutation.mutateAsync({ name, args: args[0] })

  const sync = useMutation({
    mutationKey: ["sync", url],
    mutationFn: async () => {
      qc.setQueryData(workspaceKey(url), await fetchSyncedWorkspace(cfg))
    },
  })

  async function refresh(opts?: { sync?: boolean }) {
    if (opts?.sync) {
      await sync.mutateAsync()
      await qc.invalidateQueries({ queryKey: ["diff", url] })
      return
    }
    await invalidate()
  }

  return { cfg, busy, run, refresh, invalidate }
}

/**
 * useOpPending is true while the op `name` runs, optionally only for matching args, so a
 * button shows its own loading state whether it was clicked or reached by key.
 */
export function useOpPending<K extends OpName>(
  url: string,
  name: K,
  match?: (args: OpArgs<K>) => boolean,
): boolean {
  const running = useMutationState({
    filters: { mutationKey: opKey(url), status: "pending" },
    select: (m) => m.state.variables as { name: OpName; args: unknown } | undefined,
  })
  return running.some((v) => v?.name === name && (!match || match(v.args as OpArgs<K>)))
}

/** useRefreshing is true while a manual (synced) refresh runs, not during background polls. */
export function useRefreshing(url: string | undefined): boolean {
  return useIsMutating({ mutationKey: ["sync", url] }) > 0
}

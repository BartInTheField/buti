import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  fetchDiff,
  fetchWorkspace,
  loadConfig,
  opAmend,
  opCommit,
  opMove,
  opUncommit,
  type ApiConfig,
  type Placement,
} from "./api"

// The CLI and repository errors do not change on a retry. Refresh refetches.
const noRetry = { retry: false }

export function useApiConfig() {
  return useQuery({
    queryKey: ["api-config"],
    queryFn: loadConfig,
    staleTime: Infinity,
    ...noRetry,
  })
}

export function useWorkspace(url: string | undefined, token: string | undefined) {
  return useQuery({
    queryKey: ["workspace", url],
    queryFn: () => fetchWorkspace({ url: url ?? "", token: token ?? "" }),
    enabled: Boolean(url),
    refetchInterval: 5_000,
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
    ...noRetry,
  })
}

function cfgFrom(url: string, token: string): ApiConfig {
  return { url, token }
}

export function useWorkspaceOps(url: string | undefined, token: string | undefined) {
  const qc = useQueryClient()
  const enabled = Boolean(url && token)

  async function invalidate() {
    await qc.invalidateQueries({ queryKey: ["workspace", url] })
    await qc.invalidateQueries({ queryKey: ["diff", url] })
  }

  const commit = useMutation({
    mutationFn: (args: {
      changes?: string[]
      message?: string
      placement: Placement
    }) => opCommit(cfgFrom(url!, token!), args),
    onSuccess: invalidate,
  })
  const amend = useMutation({
    mutationFn: (args: { target: string; changes?: string[] }) =>
      opAmend(cfgFrom(url!, token!), args),
    onSuccess: invalidate,
  })
  const move = useMutation({
    mutationFn: (args: { sources: string[]; placement: Placement }) =>
      opMove(cfgFrom(url!, token!), args),
    onSuccess: invalidate,
  })
  const uncommit = useMutation({
    mutationFn: (args: { sources: string[] }) =>
      opUncommit(cfgFrom(url!, token!), args),
    onSuccess: invalidate,
  })

  const busy =
    commit.isPending || amend.isPending || move.isPending || uncommit.isPending

  return { enabled, busy, commit, amend, move, uncommit, invalidate }
}

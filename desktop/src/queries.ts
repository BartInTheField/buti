import { useQuery } from "@tanstack/react-query"
import { fetchStatus, loadConfig } from "./api"

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

export function useWorkspaceStatus(url: string | undefined, token: string | undefined) {
  return useQuery({
    queryKey: ["status", url],
    queryFn: () => fetchStatus({ url: url ?? "", token: token ?? "" }),
    enabled: Boolean(url),
    ...noRetry,
  })
}

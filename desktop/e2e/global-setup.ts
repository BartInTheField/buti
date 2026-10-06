import { execFileSync, spawn, type ChildProcess } from "node:child_process"
import { mkdtempSync } from "node:fs"
import { tmpdir } from "node:os"
import path from "node:path"

// Builds the test repository (internal/testrepo), serves the API on it with
// `buti desktop --serve`, and starts Vite against that API. Needs `but` and Go on PATH.

const root = path.resolve(import.meta.dirname, "..", "..")
const desktop = path.resolve(import.meta.dirname, "..")

// Ports well away from Tauri's 1420 and common dev servers, so the e2e can run next to a
// `npm run dev` or another checkout's e2e. BUTI_E2E_PORT moves Vite (the conflicts spec
// takes the next port); BUTI_E2E_API_PORT pins the API, which otherwise takes a free port.
export const vitePort = Number(process.env.BUTI_E2E_PORT ?? 47_310)
export const apiPort = Number(process.env.BUTI_E2E_API_PORT ?? 0)

export function hasBut(): boolean {
  try {
    execFileSync("but", ["--version"], { stdio: "ignore" })
    return true
  } catch {
    return false
  }
}

/** fixtureEnv parses the `env HOME=... go run ...` line mkrepo prints into an env map. */
export function fixtureEnv(out: string): { env: Record<string, string>; repo: string } {
  const line = out.split("\n").find((l) => l.startsWith("env ")) ?? ""
  const env: Record<string, string> = {}
  for (const tok of line.split(" ")) {
    const m = /^([A-Z_]+)=(.*)$/.exec(tok)
    if (m) env[m[1]] = m[2]
  }
  const repo = line.split(" -C ")[1]?.trim() ?? ""
  return { env, repo }
}

export function waitFor(proc: ChildProcess, stream: "stdout" | "stderr", re: RegExp, ms: number) {
  return new Promise<RegExpExecArray>((resolve, reject) => {
    let buf = ""
    const timer = setTimeout(() => reject(new Error(`timed out waiting for ${re}: ${buf}`)), ms)
    proc[stream]!.on("data", (d: Buffer) => {
      buf += d.toString()
      const m = re.exec(buf)
      if (m) {
        clearTimeout(timer)
        resolve(m)
      }
    })
    proc.on("exit", (code) => reject(new Error(`exited ${code}: ${buf}`)))
  })
}

/**
 * serve starts `buti desktop --serve` on repo and Vite on vitePort against it. extraEnv is
 * added to the API's environment. The identity is testrepo.Env's: the isolated HOME has no
 * git config to commit with.
 */
export async function serve(opts: {
  bin: string
  repo: string
  env: Record<string, string>
  vitePort: number
  apiPort?: number
  extraEnv?: Record<string, string>
}): Promise<{ url: string; token: string; stop: () => void }> {
  const procs: ChildProcess[] = []
  const stop = () => procs.forEach((p) => p.kill())
  try {
    const api = spawn(opts.bin, ["-C", opts.repo, "desktop", "--serve", "--port", String(opts.apiPort ?? 0)], {
      env: {
        ...process.env,
        ...opts.env,
        GIT_CONFIG_NOSYSTEM: "1",
        GIT_AUTHOR_NAME: "Ada Lovelace",
        GIT_AUTHOR_EMAIL: "ada@example.com",
        GIT_COMMITTER_NAME: "Ada Lovelace",
        GIT_COMMITTER_EMAIL: "ada@example.com",
        ...opts.extraEnv,
      },
      stdio: ["ignore", "pipe", "pipe"],
    })
    procs.push(api)
    const [, url, token] = await waitFor(
      api,
      "stderr",
      /buti desktop: (http:\/\/\S+)\s+buti desktop: token (\w+)/,
      20_000,
    )
    const vite = spawn("npx", ["vite", "--port", String(opts.vitePort), "--strictPort"], {
      cwd: desktop,
      env: { ...process.env, VITE_BUTI_API_URL: url, VITE_BUTI_API_TOKEN: token },
      stdio: ["ignore", "pipe", "pipe"],
    })
    procs.push(vite)
    await waitFor(vite, "stdout", /ready in/i, 30_000)
    return { url, token, stop }
  } catch (err) {
    stop()
    throw err
  }
}

export default async function globalSetup() {
  if (!hasBut()) {
    process.env.BUTI_E2E_SKIP = "`but` is not on PATH"
    return
  }
  const tmp = mkdtempSync(path.join(tmpdir(), "buti-desktop-e2e-"))
  const out = execFileSync("go", ["run", "./internal/testrepo/mkrepo", tmp], {
    cwd: root,
    encoding: "utf8",
  })
  const { env, repo } = fixtureEnv(out)
  const bin = path.join(tmp, "buti")
  execFileSync("go", ["build", "-o", bin, "./cmd/buti"], { cwd: root })

  const { url, token, stop } = await serve({ bin, repo, env, vitePort, apiPort })
  // Specs that change the repository snapshot it first and restore it through the API.
  process.env.BUTI_E2E_API_URL = url
  process.env.BUTI_E2E_API_TOKEN = token
  process.env.BUTI_E2E_REPO = repo
  process.env.BUTI_E2E_BIN = bin
  return async () => stop()
}

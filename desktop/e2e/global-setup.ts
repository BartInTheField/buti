import { execFileSync, spawn, type ChildProcess } from "node:child_process"
import { mkdtempSync } from "node:fs"
import { tmpdir } from "node:os"
import path from "node:path"

// Builds the test repository (internal/testrepo), serves the API on it with
// `buti desktop --serve`, and starts Vite against that API. Needs `but` and Go on PATH.

const root = path.resolve(import.meta.dirname, "..", "..")
const desktop = path.resolve(import.meta.dirname, "..")
export const vitePort = 1421

function hasBut(): boolean {
  try {
    execFileSync("but", ["--version"], { stdio: "ignore" })
    return true
  } catch {
    return false
  }
}

/** fixtureEnv parses the `env HOME=... go run ...` line mkrepo prints into an env map. */
function fixtureEnv(out: string): { env: Record<string, string>; repo: string } {
  const line = out.split("\n").find((l) => l.startsWith("env ")) ?? ""
  const env: Record<string, string> = {}
  for (const tok of line.split(" ")) {
    const m = /^([A-Z_]+)=(.*)$/.exec(tok)
    if (m) env[m[1]] = m[2]
  }
  const repo = line.split(" -C ")[1]?.trim() ?? ""
  return { env, repo }
}

function waitFor(proc: ChildProcess, stream: "stdout" | "stderr", re: RegExp, ms: number) {
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

  const procs: ChildProcess[] = []
  const stop = () => procs.forEach((p) => p.kill())
  try {
    const api = spawn(bin, ["-C", repo, "desktop", "--serve"], {
      env: { ...process.env, ...env },
      stdio: ["ignore", "pipe", "pipe"],
    })
    procs.push(api)
    const [, url, token] = await waitFor(
      api,
      "stderr",
      /buti desktop: (http:\/\/\S+)\s+buti desktop: token (\w+)/,
      20_000,
    )

    const vite = spawn("npx", ["vite", "--port", String(vitePort), "--strictPort"], {
      cwd: desktop,
      env: { ...process.env, VITE_BUTI_API_URL: url, VITE_BUTI_API_TOKEN: token },
      stdio: ["ignore", "pipe", "pipe"],
    })
    procs.push(vite)
    await waitFor(vite, "stdout", /ready in/i, 30_000)
  } catch (err) {
    stop()
    throw err
  }

  process.env.BUTI_E2E_REPO = repo
  return async () => stop()
}

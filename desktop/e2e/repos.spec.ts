import { execFileSync } from "node:child_process"
import { existsSync, mkdirSync, mkdtempSync } from "node:fs"
import { tmpdir } from "node:os"
import path from "node:path"
import { expect, test, type Page } from "@playwright/test"
import { apiPort as sharedApiPort, fixtureEnv, serve, vitePort as sharedVitePort } from "./global-setup.js"

// Opening repositories: the window is not started in one the way the TUI is. Its own API
// runs without -C from a folder that is not a repository, and its own test repository, so
// the shared fixture the other specs use is left alone.

const shots = path.join(process.env.BUTI_SHOTS ?? "/tmp/buti-desktop-shots", "repos")
mkdirSync(shots, { recursive: true })

const root = path.resolve(import.meta.dirname, "..", "..")
// Two past the shared fixture's ports: the conflicts spec takes the next one.
const vitePort = sharedVitePort + 2
const apiPort = sharedApiPort && sharedApiPort + 2

test.skip(() => Boolean(process.env.BUTI_E2E_SKIP), process.env.BUTI_E2E_SKIP)
test.describe.configure({ mode: "serial" })
test.use({ baseURL: `http://127.0.0.1:${vitePort}` })

let repo = ""
let empty = ""
let bin = ""
let fxEnv: Record<string, string> = {}
let stop = () => {}

test.beforeAll(async () => {
  test.setTimeout(180_000)
  const tmp = mkdtempSync(path.join(tmpdir(), "buti-desktop-repos-"))
  const out = execFileSync("go", ["run", "./internal/testrepo/mkrepo", path.join(tmp, "fixture")], {
    cwd: root,
    encoding: "utf8",
  })
  const fx = fixtureEnv(out)
  repo = fx.repo
  empty = path.join(tmp, "not-a-repo")
  mkdirSync(empty)

  fxEnv = fx.env
  bin = process.env.BUTI_E2E_BIN ?? path.join(tmp, "buti")
  if (!existsSync(bin)) execFileSync("go", ["build", "-o", bin, "./cmd/buti"], { cwd: root })
  const served = await serve({ bin, cwd: empty, env: fx.env, vitePort, apiPort })
  stop = served.stop
})

test.afterAll(() => stop())

async function shot(page: Page, name: string) {
  await page.waitForFunction(() => document.getAnimations().every((a) => a.playState !== "running"))
  await page.screenshot({ path: `${shots}/${name}.png` })
}

/** openByPath goes through the browser fallback: no native folder dialog outside Tauri. */
async function openByPath(page: Page, dir: string) {
  const dialog = page.getByRole("dialog")
  await expect(dialog.getByText("Open repository")).toBeVisible()
  await dialog.locator("input").fill(dir)
  await dialog.getByRole("button", { name: "Open" }).click()
}

test("a window started outside a repository asks for one", async ({ page }) => {
  await page.goto("/")
  const picker = page.getByTestId("repo-picker")
  await expect(picker.getByText("Open a repository")).toBeVisible()
  // No -C: the working directory not being a workspace is not an error worth shouting about.
  await expect(picker.getByText("Could not read workspace")).toHaveCount(0)
  await expect(page.getByTestId("recent-repos")).toHaveCount(0)
  await shot(page, "picker-empty")

  // A folder that is not a workspace fails and keeps the picker.
  await picker.getByRole("button", { name: /Choose folder/ }).click()
  await openByPath(page, empty)
  await expect(page.getByText(/No git repository|Setup required|GitButler/i).last()).toBeVisible()
  await expect(picker).toBeVisible()

  await page.locator("body").press("ControlOrMeta+o")
  await openByPath(page, repo)
  await expect(page.getByTestId("repo-menu")).toHaveText(path.basename(repo))
  await expect(page.locator('[data-testid="branch-card"][data-branch="auth"]')).toBeVisible()
})

test("the header switches repository", async ({ page }) => {
  // The browser profile is fresh per test: record the repository as the previous test's window did.
  await page.addInitScript((dir) => localStorage.setItem("buti.recentRepos", JSON.stringify([dir])), repo)
  await page.goto("/")
  // The API already serves repo (the previous test switched it), so it simply loads.
  await expect(page.getByTestId("repo-menu")).toHaveText(path.basename(repo))

  await page.getByTestId("repo-menu").click()
  const menu = page.getByRole("menu")
  await expect(menu.getByText(repo)).toBeVisible()
  await expect(menu.getByRole("menuitem", { name: /Open folder/ })).toBeVisible()
  await shot(page, "header-menu")

  // A failed switch keeps the workspace on screen.
  await menu.getByRole("menuitem", { name: /Open folder/ }).click()
  await openByPath(page, empty)
  await expect(page.getByTestId("repo-menu")).toHaveText(path.basename(repo))
  await expect(page.locator('[data-testid="branch-card"][data-branch="auth"]')).toBeVisible()

  // The palette offers it too.
  await page.locator("body").press("ControlOrMeta+k")
  await expect(page.getByRole("option", { name: /Open repository/ })).toBeVisible()
  await page.keyboard.press("Escape")
})

test("a window started outside a repository reopens the last one", async ({ page }) => {
  test.setTimeout(120_000)
  // A fresh API without -C, on the next ports: it starts in the empty folder, not on repo.
  const port = vitePort + 1
  const next = await serve({ bin, cwd: empty, env: fxEnv, vitePort: port, apiPort: apiPort && apiPort + 1 })
  try {
    // localStorage is per origin: seed it once, as an earlier session on this port would have.
    await page.addInitScript((dirs) => {
      if (localStorage.getItem("buti.recentRepos") === null) {
        localStorage.setItem("buti.recentRepos", JSON.stringify(dirs))
      }
    }, ["/no/such/repo", repo])
    await page.goto(`http://127.0.0.1:${port}/`)
    // The newest entry is gone: it is dropped and the picker shows, without trying the next one.
    await expect(page.getByTestId("repo-picker")).toBeVisible()
    await expect(page.getByTestId("recent-repos").getByText(repo)).toBeVisible()
    await expect(page.getByTestId("recent-repos").getByText("/no/such/repo")).toHaveCount(0)
    await shot(page, "picker-recent")

    await page.reload()
    await expect(page.getByTestId("repo-menu")).toHaveText(path.basename(repo))
    await expect(page.locator('[data-testid="branch-card"][data-branch="auth"]')).toBeVisible()
  } finally {
    next.stop()
  }
})

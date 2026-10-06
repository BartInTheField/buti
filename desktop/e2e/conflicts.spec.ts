import { execFileSync } from "node:child_process"
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import path from "node:path"
import { expect, test, type Locator, type Page } from "@playwright/test"
import { apiPort as sharedApiPort, fixtureEnv, serve, vitePort as sharedVitePort } from "./global-setup.js"

// Conflicts and edit mode, against its own test repository with a conflicted commit
// (`mkrepo -conflict`, testrepo.Repo.Conflict) and its own API and Vite, so the shared
// fixture the other specs use is left alone. Files "opened" are logged, not opened.

const shots = path.join(process.env.BUTI_SHOTS ?? "/tmp/buti-desktop-shots", "conflicts")
mkdirSync(shots, { recursive: true })

const root = path.resolve(import.meta.dirname, "..", "..")
// The port after the shared fixture's, so both servers can run at once.
const vitePort = sharedVitePort + 1
const apiPort = sharedApiPort && sharedApiPort + 1

test.skip(() => Boolean(process.env.BUTI_E2E_SKIP), process.env.BUTI_E2E_SKIP)
test.describe.configure({ mode: "serial" })
test.use({ baseURL: `http://127.0.0.1:${vitePort}` })

let repo = ""
let openLog = ""
let stop = () => {}

test.beforeAll(async () => {
  test.setTimeout(180_000)
  const tmp = mkdtempSync(path.join(tmpdir(), "buti-desktop-conflicts-"))
  const out = execFileSync("go", ["run", "./internal/testrepo/mkrepo", "-conflict", tmp], {
    cwd: root,
    encoding: "utf8",
  })
  const fx = fixtureEnv(out)
  repo = fx.repo
  openLog = path.join(tmp, "opened.log")
  const opener = path.join(tmp, "open.sh")
  writeFileSync(opener, `#!/bin/sh\necho "$@" >> '${openLog}'\n`)
  chmodSync(opener, 0o755)

  const bin = process.env.BUTI_E2E_BIN ?? path.join(tmp, "buti")
  if (!existsSync(bin)) execFileSync("go", ["build", "-o", bin, "./cmd/buti"], { cwd: root })
  const served = await serve({
    bin,
    repo,
    env: fx.env,
    vitePort,
    apiPort,
    extraEnv: { BUTI_DESKTOP_OPEN: opener },
  })
  stop = served.stop
})

test.afterAll(() => stop())

async function settle(page: Page) {
  await page.waitForFunction(() => document.getAnimations().every((a) => a.playState !== "running"))
}

function opened(): string[] {
  return existsSync(openLog) ? readFileSync(openLog, "utf8").trim().split("\n") : []
}

async function rects(l: Locator) {
  return l.evaluateAll((els) => els.map((el) => JSON.stringify(el.getBoundingClientRect())))
}

function conflictedRow(page: Page) {
  return page.getByTestId("commit-row").filter({ hasText: "Start a changelog" })
}

test.beforeEach(async ({ page }) => {
  await page.goto("/")
  await expect(page.getByTestId("branch-card").or(page.getByTestId("edit-mode")).first()).toBeVisible()
})

test("a conflicted commit shows ✗ in its lane, with a tooltip and no layout shift", async ({ page }) => {
  const row = conflictedRow(page)
  await expect(row.getByTestId("conflict-badge")).toHaveText("✗ Conflicted")
  // Only that commit is conflicted.
  await expect(page.getByTestId("conflict-badge")).toHaveCount(1)

  const before = await rects(page.getByTestId("commit-row"))
  await row.getByTestId("conflict-badge").hover()
  await expect(page.getByRole("tooltip")).toContainText("press e to resolve")
  expect(await rects(page.getByTestId("commit-row"))).toEqual(before)
  await settle(page)
  await page.screenshot({ path: `${shots}/conflicted.png` })

  await row.click({ button: "right" })
  await expect(page.getByRole("menu").getByText("Resolve in edit mode")).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(page.getByRole("menu")).toHaveCount(0)
})

test("e enters edit mode; x cancels after confirming", async ({ page }) => {
  await conflictedRow(page).click()
  await page.keyboard.press("e")
  const edit = page.getByTestId("edit-mode")
  await expect(edit.getByText("You are editing commit")).toBeVisible()
  await expect(edit.getByTestId("edited-commit")).toContainText("Start a changelog")
  const file = edit.getByTestId("conflict-file").filter({ hasText: "CHANGELOG.md" })
  await expect(file.getByText("Conflicted")).toBeVisible()
  await expect(edit.getByTestId("conflict-text")).toContainText("<<<<<<<")

  await page.keyboard.press("x")
  const dialog = page.getByRole("alertdialog")
  await expect(dialog.getByText("Leave edit mode without saving?")).toBeVisible()
  await settle(page)
  await page.screenshot({ path: `${shots}/cancel-confirm.png` })
  await dialog.getByRole("button", { name: "Leave" }).click()
  await expect(edit).toHaveCount(0)
  await expect(conflictedRow(page).getByTestId("conflict-badge")).toBeVisible()
})

test("edit mode: open files, resolve, save and exit", async ({ page }) => {
  await conflictedRow(page).click()
  await page.keyboard.press("e")
  const edit = page.getByTestId("edit-mode")
  await expect(edit.getByText("Conflicted", { exact: true })).toBeVisible()
  await expect(edit.getByTestId("conflict-text")).toContainText("Current commit: Start a changelog")
  await settle(page)
  await page.screenshot({ path: `${shots}/edit-mode.png` })
  // A reload keeps edit mode, and the edited commit's card.
  await page.emulateMedia({ colorScheme: "dark" })
  await page.reload()
  await expect(edit.getByTestId("edited-commit")).toContainText("Start a changelog")
  await settle(page)
  await page.screenshot({ path: `${shots}/edit-mode-dark.png` })
  await page.emulateMedia({ colorScheme: "light" })

  const changelog = path.join(repo, "CHANGELOG.md")
  await page.keyboard.press("o")
  await expect.poll(opened).toEqual([changelog])
  await page.keyboard.press("Enter")
  await expect.poll(opened).toHaveLength(2)
  await edit.getByTestId("conflict-file").first().dblclick()
  await expect.poll(opened).toHaveLength(3)

  // Saving with markers left asks first.
  await edit.getByRole("button", { name: /Save and exit/ }).click()
  const dialog = page.getByRole("alertdialog")
  await expect(dialog.getByText("Save with conflict markers left?")).toBeVisible()
  await expect(dialog).toContainText("CHANGELOG.md")
  await settle(page)
  await page.screenshot({ path: `${shots}/save-confirm.png` })
  await dialog.getByRole("button", { name: "Cancel" }).click()
  await expect(dialog).toHaveCount(0)

  // Fix the file as an editor would; the view picks it up on its next poll.
  writeFileSync(changelog, "# Changelog\n\n- Health endpoint\n- Token auth\n")
  await expect(edit.getByText("Resolved", { exact: true })).toBeVisible({ timeout: 15_000 })
  await expect(edit.getByTestId("conflict-text")).not.toContainText("<<<<<<<", { timeout: 10_000 })
  await expect(edit.getByRole("button", { name: /Open conflicted files/ })).toBeDisabled()
  await settle(page)
  await page.screenshot({ path: `${shots}/resolved.png` })

  await page.keyboard.press("e")
  await expect(edit).toHaveCount(0)
  await expect(conflictedRow(page)).toBeVisible()
  await expect(page.getByTestId("conflict-badge")).toHaveCount(0)
  await settle(page)
  await page.screenshot({ path: `${shots}/done.png` })
})

test("o and O open an uncommitted file", async ({ page }) => {
  const before = opened().length
  const file = page.getByTestId("file-row").filter({ hasText: "README.md" })
  await file.click()
  await page.keyboard.press("O")
  await expect.poll(() => opened().slice(before)).toEqual([path.join(repo, "README.md")])

  await file.click({ button: "right" })
  const menu = page.getByRole("menu")
  await expect(menu.getByText("Open with default app")).toBeVisible()
  await settle(page)
  await page.screenshot({ path: `${shots}/open-menu.png` })
  await menu.getByText("Open in editor").click()
  await expect.poll(() => opened().length).toBe(before + 2)
})

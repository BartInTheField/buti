import { mkdirSync, readFileSync } from "node:fs"
import path from "node:path"
import { expect, test, type Page } from "@playwright/test"

// The details pane and review comments against the real fixture: line cursor, ranges,
// hunks and hunk marks, f / F, full screen and the file tree, copy, go to, and comments
// written to the same store the TUI and `buti review` read.

const shots = path.join(process.env.BUTI_SHOTS ?? "/tmp/buti-desktop-shots", "details")
mkdirSync(shots, { recursive: true })

test.skip(() => Boolean(process.env.BUTI_E2E_SKIP), process.env.BUTI_E2E_SKIP)

async function settle(page: Page) {
  await page.waitForFunction(() => document.getAnimations().every((a) => a.playState !== "running"))
}

function storedComments(): { id: string; body: string; status: string; anchor: { line: number; end_line: number } }[] {
  const file = path.join(process.env.BUTI_E2E_REPO ?? "", ".git", "buti", "review.json")
  try {
    return JSON.parse(readFileSync(file, "utf8")).comments
  } catch {
    return []
  }
}

/** api calls the desktop API from the page, with the config the app itself uses. */
async function api(page: Page, route: string, body?: unknown) {
  return page.evaluate(
    async ({ route, body }) => {
      // The Vite dev server serves the app's own modules; a variable keeps tsc off the path.
      const src = "/src/api.ts"
      const m = (await import(src)) as { loadConfig: () => Promise<{ url: string; token: string }> }
      const cfg = await m.loadConfig()
      const res = await fetch(cfg.url + route, {
        method: body === undefined ? "GET" : "POST",
        headers: { Authorization: `Bearer ${cfg.token}`, "Content-Type": "application/json" },
        body: body === undefined ? undefined : JSON.stringify(body),
      })
      return res.json()
    },
    { route, body },
  )
}

async function openFile(page: Page, name: string) {
  await page.locator(`[data-testid="file-row"][title="${name}"]`).click()
  await expect(page.getByTestId("diff-pane").getByRole("heading")).toHaveText(name)
  await expect(page.getByTestId("diff-line").first()).toBeVisible()
}

const pane = (page: Page) => page.getByTestId("diff-scroll")
const cursorLine = (page: Page) => page.locator("[data-cursor=true]")

test.beforeEach(async ({ page }) => {
  await page.goto("/")
  await expect(page.getByTestId("branch-card").first()).toBeVisible()
})

test.afterAll(async ({ browser }) => {
  // Leave the store as we found it for the other specs.
  const page = await browser.newPage()
  await page.goto("/")
  for (const c of storedComments()) await api(page, "/comments/delete", { id: c.id })
  await page.close()
})

test("diff is highlighted, and the line cursor moves with keys and clicks", async ({ page }) => {
  await openFile(page, "src/util/strings.go")
  // Chroma tokens arrive after the plain diff; the keyword gets its class.
  await expect(pane(page).locator(".tok-k").first()).toHaveText("package")
  const before = await page.getByTestId("diff-line").first().boundingBox()

  await page.getByTestId("diff-line").nth(2).click()
  await expect(cursorLine(page)).toContainText("func Reverse")
  await page.keyboard.press("j")
  await expect(cursorLine(page)).toContainText("r := []rune(s)")
  await page.keyboard.press("k")
  await page.keyboard.press("k")
  await expect(cursorLine(page)).toContainText("+")

  // A range: v, then two lines down; shift-click extends too.
  await page.keyboard.press("v")
  await page.keyboard.press("j")
  await page.keyboard.press("j")
  await expect(pane(page).locator("[data-range]")).toHaveCount(3)
  expect(await page.getByTestId("diff-line").first().boundingBox()).toEqual(before)
  await settle(page)
  await page.screenshot({ path: `${shots}/range.png` })
  await page.keyboard.press("Escape")
  await expect(pane(page).locator("[data-range]")).toHaveCount(0)

  // Dragging with the mouse selects the lines it covers, like v.
  const lines = page.getByTestId("diff-line")
  const from = (await lines.nth(2).boundingBox())!
  const to = (await lines.nth(5).boundingBox())!
  await page.mouse.move(from.x + 80, from.y + from.height / 2)
  await page.mouse.down()
  await page.mouse.move(to.x + 80, to.y + to.height / 2, { steps: 6 })
  await page.mouse.up()
  await expect(pane(page).locator("[data-range]")).toHaveCount(4)
  await expect(cursorLine(page)).toBeVisible()
  expect(await page.getByTestId("diff-line").first().boundingBox()).toEqual(before)
  await settle(page)
  await page.screenshot({ path: `${shots}/range-drag.png` })
  // A plain click afterwards is a single line again.
  await lines.nth(3).click()
  await expect(pane(page).locator("[data-range]")).toHaveCount(0)
})

test("the first diff load shows a skeleton in the diff's shape", async ({ page }) => {
  // Hold the first diff back, so the skeleton stays up long enough to look at.
  let release = () => {}
  const held = new Promise<void>((r) => (release = r))
  await page.route((url) => url.pathname === "/diff", async (route) => {
    await held
    await route.continue()
  })
  await page.goto("/")
  const skeleton = page.getByTestId("diff-skeleton")
  await expect(skeleton).toBeVisible()
  await expect(pane(page).getByText("Loading diff")).toHaveCount(0)
  // No settle: the skeleton's pulse never finishes.
  await page.screenshot({ path: `${shots}/diff-skeleton.png` })
  const header = await skeleton.getByTestId("diff-skeleton-file").first().boundingBox()
  release()
  await expect(page.getByTestId("diff-line").first()).toBeVisible()
  await expect(skeleton).toHaveCount(0)
  // The first file header lands where the skeleton's was.
  const first = await pane(page).locator('[data-index="0"]').boundingBox()
  expect(first?.y).toBe(header?.y)
  expect(first?.height).toBe(header?.height)
})

test("the diff never blanks while it refetches", async ({ page }) => {
  await openFile(page, "README.md")
  const counts: number[] = []
  await page.getByRole("button", { name: "Refresh" }).click()
  for (let i = 0; i < 20; i++) {
    counts.push(await page.getByTestId("diff-line").count())
    await page.waitForTimeout(25)
  }
  expect(Math.min(...counts)).toBeGreaterThan(0)
})

test("comment on a range, edit, resolve, hide resolved and delete", async ({ page }) => {
  await openFile(page, "src/util/strings.go")
  await page.getByTestId("diff-line").nth(2).click()
  await page.keyboard.press("v")
  await page.keyboard.press("j")
  await page.keyboard.press("C")
  const composer = page.getByTestId("comment-composer")
  await expect(composer).toBeVisible()
  await expect(composer).toContainText("src/util/strings.go lines 3–4")
  await composer.getByRole("textbox").fill("[must-fix] Reverse breaks on combining characters")
  await settle(page)
  await page.screenshot({ path: `${shots}/composer.png` })
  await page.keyboard.press("ControlOrMeta+s")
  await expect(composer).toBeHidden()

  const thread = page.getByTestId("comment-thread")
  await expect(thread).toContainText("Reverse breaks on combining characters")
  await expect(thread).toContainText("lines 3–4")
  await expect(page.getByTestId("open-comments")).toHaveText("1")
  // Same store as the TUI and `buti review list`.
  const stored = storedComments()
  expect(stored).toHaveLength(1)
  expect(stored[0].anchor).toMatchObject({ line: 3, end_line: 4 })
  await settle(page)
  await page.screenshot({ path: `${shots}/comment-thread.png` })

  // The cursor, still on the range's last line, stops on the comment next; e edits it.
  await pane(page).focus()
  await page.keyboard.press("j")
  await expect(thread).toHaveClass(/ring-2/)
  await page.keyboard.press("e")
  await expect(composer.getByRole("textbox")).toHaveValue(/combining/)
  await composer.getByRole("textbox").fill("[nit] Reverse works on runes, fine")
  await composer.getByRole("button", { name: "Save" }).click()
  await expect(thread).toContainText("[nit] Reverse works on runes, fine")

  // x resolves it, z hides resolved ones, z shows them again, x reopens.
  await pane(page).focus()
  await page.keyboard.press("x")
  await expect(thread).toHaveAttribute("data-state", "resolved")
  await expect(page.getByTestId("open-comments")).toHaveClass(/invisible/)
  await settle(page)
  await page.screenshot({ path: `${shots}/comment-resolved.png` })
  await page.keyboard.press("z")
  await expect(thread).toHaveCount(0)
  await page.keyboard.press("z")
  await expect(thread).toHaveAttribute("data-state", "resolved")
  await thread.getByRole("button", { name: /Reopen/ }).click()
  await expect(thread).toHaveAttribute("data-state", "open")

  // d deletes after a confirm.
  await thread.click()
  await page.keyboard.press("d")
  const confirm = page.getByRole("alertdialog")
  await expect(confirm).toContainText("Delete comment on src/util/strings.go lines 3–4?")
  await settle(page)
  await page.screenshot({ path: `${shots}/comment-delete.png` })
  await confirm.getByRole("button", { name: "Delete" }).click()
  await expect(thread).toHaveCount(0)
  expect(storedComments()).toHaveLength(0)
})

test("Review comments… jumps to an open comment", async ({ page }) => {
  await api(page, "/comments", {
    anchor: { kind: "unassigned", path: "README.md", side: "new", line: 6, end_line: 6, line_text: "## Usage" },
    body: "Document the flags too",
    author: "agent",
  })
  await page.getByTestId("branch-card").filter({ hasText: "fix-typo" }).getByRole("button").first().click()
  await page.keyboard.press("ControlOrMeta+k")
  await page.getByRole("dialog").getByPlaceholder(/type to search/).fill("review comments")
  await page.keyboard.press("Enter")
  const picker = page.getByRole("dialog")
  // Located again: "## Usage" is on line 5, so that is where the comment is now.
  await expect(picker).toContainText("README.md line 5")
  await expect(picker).toContainText("agent")
  await settle(page)
  await page.screenshot({ path: `${shots}/comments-picker.png` })
  await page.keyboard.press("Enter")
  await expect(page.getByTestId("diff-pane").getByRole("heading")).toHaveText("README.md")
  const thread = page.getByTestId("comment-thread")
  await expect(thread).toContainText("Document the flags too")
  await expect(thread).toHaveClass(/ring-2/)
  await settle(page)
  await page.screenshot({ path: `${shots}/comment-agent.png` })
})

test("hunk marks: space marks the cursor's hunk and commit acts on it", async ({ page }) => {
  await page.getByRole("button", { name: /^Unstaged/ }).click()
  await expect(page.getByTestId("hunk-header").first()).toBeVisible()
  await page.getByTestId("hunk-header").filter({ hasText: "@@ -6,5 +6,5 @@" }).click()
  await page.keyboard.press(" ")
  await expect(page.getByTestId("hunk-header").filter({ hasText: "@@ -6,5 +6,5 @@" })).toContainText("marked")
  await expect(page.getByTestId("marks")).toContainText("hunk @@ -6,5 +6,5 @@ of src/server.go marked")
  await settle(page)
  await page.screenshot({ path: `${shots}/hunk-marked.png` })

  // c enters target mode with the marked hunk as the source; / lists the targets.
  await page.keyboard.press("c")
  await expect(page.getByTestId("target-desc")).toHaveText("Click a target for hunk of src/server.go")
  const empty = page.locator('[data-testid="branch-card"][data-branch="empty"]')
  await empty.locator("button").first().hover()
  await expect(page.getByTestId("target-desc")).toHaveText("Commit hunk of src/server.go to branch empty")
  await settle(page)
  await page.screenshot({ path: `${shots}/hunk-target.png` })
  await page.keyboard.press("/")
  const list = page.getByRole("dialog")
  await expect(list.getByRole("option", { name: /empty/ })).toBeVisible()
  await settle(page)
  await page.screenshot({ path: `${shots}/hunk-target-picker.png` })
  await list.getByRole("combobox").fill("empty")
  await list.getByRole("option", { name: /empty/ }).click()
  const composer = page.getByRole("dialog")
  await expect(composer.getByText("Commit hunk of src/server.go to branch empty.")).toBeVisible()
  await composer.locator("textarea").fill("Listen on 9090")
  await page.keyboard.press("ControlOrMeta+Enter")
  await expect(empty).toContainText("Listen on 9090")
  await expect(page.locator(`[data-testid="file-row"][title="src/server.go"]`)).toHaveCount(0)

  // Put the fixture back for the other specs.
  expect((await api(page, "/ops/undo", {})).ok).toBe(true)
  await page.getByRole("button", { name: "Refresh" }).click()
  await expect(page.locator(`[data-testid="file-row"][title="src/server.go"]`)).toHaveCount(1)
  await expect(empty).not.toContainText("Listen on 9090")
})

test("x on the cursor's hunk discards only that hunk", async ({ page }) => {
  await page.getByRole("button", { name: /^Unstaged/ }).click()
  await page.getByTestId("hunk-header").filter({ hasText: "@@ -6,5 +6,5 @@" }).click()
  await page.keyboard.press("x")
  const confirm = page.getByRole("alertdialog")
  await expect(confirm).toContainText("Discard hunk of src/server.go?")
  await confirm.getByRole("button", { name: "Discard" }).click()
  await expect(page.locator(`[data-testid="file-row"][title="src/server.go"]`)).toHaveCount(0)
  await expect(page.getByTestId("file-row").filter({ hasText: "README.md" })).toHaveCount(1)

  expect((await api(page, "/ops/undo", {})).ok).toBe(true)
  await page.getByRole("button", { name: "Refresh" }).click()
  await expect(page.locator(`[data-testid="file-row"][title="src/server.go"]`)).toHaveCount(1)
})

test("a committed file uncommits with r and moves into another commit by drag", async ({ page }) => {
  await page.getByTestId("commit-row").filter({ hasText: "Bump Go version" }).click()
  await page.keyboard.press("f")
  const file = page.getByTestId("commit-file").filter({ hasText: "go.mod" })
  await file.click()
  await page.keyboard.press("r")
  await page.getByRole("button", { name: /^Unstaged/ }).hover()
  await expect(page.getByTestId("target-desc")).toHaveText("Uncommit file go.mod")
  await page.getByTestId("commit-row").filter({ hasText: "Document the health handler" }).hover()
  await expect(page.getByTestId("target-tag")).toHaveText("Move into this commit")
  await page.keyboard.press("Escape")
  await expect(page.getByTestId("target-bar")).toHaveCount(0)

  const from = (await file.boundingBox())!
  const to = (await page.getByTestId("commit-row").filter({ hasText: "Document the health handler" }).boundingBox())!
  await page.mouse.move(from.x + 20, from.y + from.height / 2)
  await page.mouse.down()
  await page.mouse.move(from.x + 40, from.y + 8, { steps: 4 })
  await page.mouse.move(to.x + 40, to.y + to.height / 2, { steps: 6 })
  await expect(page.getByTestId("drop-hint")).toHaveText("Move into this commit")
  await settle(page)
  await page.screenshot({ path: `${shots}/drag-committed-file.png` })
  await page.keyboard.press("Escape")
  await page.mouse.up()
  await page.keyboard.press("f")
})

test("f lists a commit's files, F all of them, and a committed file shows its diff", async ({ page }) => {
  const commit = page.getByTestId("commit-row").filter({ hasText: "Bump Go version" })
  await commit.click()
  await page.keyboard.press("f")
  const files = page.getByTestId("commit-file")
  await expect(files.filter({ hasText: "go.mod" })).toBeVisible()
  await files.filter({ hasText: "go.mod" }).click()
  await expect(page.getByTestId("diff-pane").getByRole("heading")).toHaveText("go.mod")
  await expect(page.getByTestId("diff-line").first()).toBeVisible()
  await page.keyboard.press("F")
  await expect.poll(() => files.count()).toBeGreaterThan(2)
  await settle(page)
  await page.screenshot({ path: `${shots}/commit-files.png` })
  await page.keyboard.press("F")
  await page.keyboard.press("f")
  await expect(files).toHaveCount(0)
})

test("full screen with the file tree, T hides it, esc leaves; d hides the pane", async ({ page }) => {
  await page.getByRole("button", { name: /^Unstaged/ }).click()
  await expect(page.getByTestId("diff-line").first()).toBeVisible()
  await page.keyboard.press("D")
  const tree = page.getByTestId("file-tree")
  await expect(tree).toBeVisible()
  await expect(page.getByTestId("branch-card").first()).not.toBeInViewport()
  await tree.getByTestId("tree-file").filter({ hasText: "strings.go" }).click()
  await expect(cursorLine(page)).toContainText("package util")
  // A shift-click on another file must not select the file names as text.
  await tree.getByTestId("tree-file").first().click({ modifiers: ["Shift"] })
  expect(await page.evaluate(() => window.getSelection()?.toString() ?? "")).toBe("")
  await expect(tree).toHaveCSS("user-select", "none")
  await settle(page)
  await page.screenshot({ path: `${shots}/full-screen.png` })
  // Folding a directory.
  await tree.getByRole("button", { name: "src/" }).click()
  await expect(tree.getByTestId("tree-file").filter({ hasText: "server.go" })).toHaveCount(0)
  await page.keyboard.press("T")
  await expect(tree).toBeHidden()
  await page.keyboard.press("Escape")
  await expect(page.getByTestId("branch-card").first()).toBeInViewport()

  await page.keyboard.press("d")
  await expect(page.getByTestId("diff-pane")).not.toBeInViewport()
  await settle(page)
  await page.screenshot({ path: `${shots}/details-hidden.png` })
  await page.keyboard.press("d")
  await expect(page.getByTestId("diff-pane")).toBeInViewport()
})

test("copy and go to", async ({ page, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"])
  await page.locator('[data-testid="branch-card"][data-branch="auth"]').getByRole("button").first().click()
  await page.keyboard.press("y")
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe("auth")

  await page.getByTestId("commit-row").filter({ hasText: "Bump Go version" }).click()
  await page.keyboard.press("Y")
  const picker = page.getByRole("dialog")
  await expect(picker.getByRole("option", { name: /Change ID/ })).toBeVisible()
  await settle(page)
  await page.screenshot({ path: `${shots}/copy-picker.png` })
  await picker.getByRole("option", { name: /Message title/ }).click()
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe("Bump Go version")

  await page.keyboard.press("/")
  const go = page.getByRole("dialog")
  await go.getByRole("combobox").fill("strings.go")
  await settle(page)
  await page.screenshot({ path: `${shots}/goto.png` })
  await page.keyboard.press("Enter")
  await expect(page.getByTestId("diff-pane").getByRole("heading")).toHaveText("src/util/strings.go")
})

test("dark mode: a dragged range shows on added lines", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" })
  await openFile(page, "src/util/strings.go")
  const added = page.locator('[data-testid="diff-line"][data-sign="+"]')
  // Earlier tests commit and discard hunks of this file, so only count on two added lines.
  const bg = (i: number) => added.nth(i).evaluate((el) => getComputedStyle(el).backgroundColor)
  const plain = await bg(0)
  const from = (await added.nth(0).boundingBox())!
  const to = (await added.nth(1).boundingBox())!
  await page.mouse.move(from.x + 80, from.y + from.height / 2)
  await page.mouse.down()
  await page.mouse.move(to.x + 80, to.y + to.height / 2, { steps: 6 })
  await page.mouse.up()
  await expect(pane(page).locator("[data-range]")).toHaveCount(2)
  // The diff's dark: tint used to win over the range's, so only the cursor line showed.
  expect(await bg(0)).not.toBe(plain)
  await settle(page)
  await page.screenshot({ path: `${shots}/dark-range-drag.png` })
})

test("dark mode: highlighted diff with a comment, full screen", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" })
  await api(page, "/comments", {
    anchor: { kind: "unassigned", path: "src/server.go", side: "new", line: 9, end_line: 9, line_text: '\treturn http.ListenAndServe(":9090", nil)' },
    body: "[question] Why move off 8080?",
  })
  // Picked up by the next poll otherwise, as the TUI's refresh tick does.
  await page.reload()
  await page.getByRole("button", { name: /^Unstaged/ }).click()
  await page.keyboard.press("D")
  // Rows are virtualized: the thread exists once its file is scrolled into view.
  await page.getByTestId("tree-file").filter({ hasText: "server.go" }).click()
  await expect(page.getByTestId("comment-thread").filter({ hasText: "Why move off 8080?" })).toBeVisible()
  await expect(pane(page).locator(".tok-k").first()).toBeVisible()
  await settle(page)
  await page.screenshot({ path: `${shots}/dark-full-screen.png` })
})

test("right-click in the diff offers the hunk's actions and the comment actions", async ({ page }) => {
  await openFile(page, "src/util/strings.go")
  await page.getByTestId("diff-line").nth(3).click({ button: "right" })
  const menu = page.getByTestId("diff-menu")
  await expect(menu).toContainText("hunk @@ -1,0 +1,9 @@ of src/util/strings.go")
  await expect(menu.getByRole("menuitem", { name: /Comment on the line/ })).toBeVisible()
  await expect(menu.getByRole("menuitem", { name: /Mark \/ unmark hunk/ })).toBeVisible()
  await expect(menu.getByRole("menuitem", { name: /^Commit…/ })).toBeVisible()
  await settle(page)
  await page.screenshot({ path: `${shots}/diff-menu.png` })
  await menu.getByRole("menuitem", { name: /Comment on the line/ }).click()
  await expect(page.getByTestId("comment-composer")).toContainText("src/util/strings.go line 4")
  await page.keyboard.press("Escape")
  await expect(page.getByTestId("comment-composer")).toBeHidden()
})

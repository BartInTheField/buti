import { mkdirSync } from "node:fs"
import { expect, test, type Page } from "@playwright/test"

const shots = `${process.env.BUTI_SHOTS ?? "/tmp/buti-desktop-shots"}/commits`
mkdirSync(shots, { recursive: true })

// The flows in the window, against the real fixture. Every test that changes the
// repository restores it afterwards, so the other specs see the fixture as built.
test.describe("commit verbs", () => {
  test.skip(() => Boolean(process.env.BUTI_E2E_SKIP), process.env.BUTI_E2E_SKIP)

  const api = process.env.BUTI_E2E_API_URL ?? ""
  const auth = { Authorization: `Bearer ${process.env.BUTI_E2E_API_TOKEN ?? ""}` }
  let snapshot = ""

  test.beforeEach(async ({ page, request }) => {
    await request.post(`${api}/exec`, { headers: auth, data: { line: "oplog snapshot" } })
    const res = await request.get(`${api}/oplog`, { headers: auth })
    snapshot = (await res.json()).entries[0].id
    await page.goto("/")
    await expect(page.getByTestId("branch-card").first()).toBeVisible()
    await expect(page.getByTestId("file-row").first()).toBeVisible()
  })

  test.afterEach(async ({ request }) => {
    const res = await request.post(`${api}/ops/oplog-restore`, { headers: auth, data: { snapshot } })
    expect(res.ok()).toBe(true)
  })

  const card = (page: Page, name: string) =>
    page.locator(`[data-testid="branch-card"][data-branch="${name}"]`)
  const commitRow = (page: Page, subject: string) =>
    page.getByTestId("commit-row").filter({ hasText: subject })
  const fileRow = (page: Page, path: string) => page.locator(`[data-testid="file-row"][title="${path}"]`)

  async function boxes(page: Page) {
    return page
      .locator('[data-testid="branch-card"], [data-testid="commit-row"], [data-testid="new-branch-lane"], [data-testid="file-row"]')
      .evaluateAll((els) => els.map((el) => JSON.stringify(el.getBoundingClientRect())))
  }

  async function settle(page: Page) {
    await page.waitForFunction(() => document.getAnimations().every((a) => a.playState !== "running"))
  }

  test("c picks a target in the lanes without moving anything, then commits", async ({ page }) => {
    await fileRow(page, "README.md").click()
    const before = await boxes(page)
    await page.keyboard.press("c")
    const bar = page.getByTestId("target-bar")
    await expect(bar).toBeVisible()
    await expect(page.getByTestId("target-desc")).toHaveText("Click a target for file README.md")
    expect(await boxes(page)).toEqual(before)

    await card(page, "fix-typo").locator("button").first().hover()
    await expect(page.getByTestId("target-desc")).toHaveText("Commit file README.md to branch fix-typo")
    await expect(page.getByTestId("target-tag")).toHaveText("Commit onto fix-typo")
    expect(await boxes(page)).toEqual(before)
    await settle(page)
    await page.screenshot({ path: `${shots}/target-commit-branch.png` })

    // Above/below on a commit, with the insertion marker.
    await commitRow(page, "Add token auth").hover()
    await expect(page.getByTestId("target-tag")).toHaveText("Commit below")
    await page.keyboard.press("a")
    await expect(page.getByTestId("target-tag")).toHaveText("Commit above")
    await expect(page.getByTestId("target-insert")).toHaveCount(1)
    expect(await boxes(page)).toEqual(before)
    await settle(page)
    await page.screenshot({ path: `${shots}/target-commit-above.png` })

    await card(page, "fix-typo").locator("button").first().click()
    const dialog = page.getByRole("dialog")
    await expect(dialog.getByText("Commit file README.md to branch fix-typo.")).toBeVisible()
    await dialog.locator("textarea").fill("Explain usage in the README")
    await dialog.getByRole("button", { name: "Commit" }).click()
    await expect(commitRow(page, "Explain usage in the README")).toHaveCount(1)
    await expect(card(page, "fix-typo").getByText("Explain usage in the README")).toBeVisible()
    await expect(fileRow(page, "README.md")).toHaveCount(0)
    await expect(bar).toHaveCount(0)
  })

  test("target mode in the dark theme", async ({ page }) => {
    await page.emulateMedia({ colorScheme: "dark" })
    await page.reload()
    await commitRow(page, "Test token auth").click()
    await page.keyboard.press("m")
    await commitRow(page, "Add users endpoint").hover()
    await expect(page.getByTestId("target-tag")).toHaveText("Move above")
    await settle(page)
    await page.screenshot({ path: `${shots}/target-move-dark.png` })
    await page.keyboard.press("Escape")
    await expect(page.getByTestId("target-bar")).toHaveCount(0)
  })

  test("escape cancels, and a drag over a commit offers a squash", async ({ page }) => {
    await fileRow(page, "README.md").click()
    await page.keyboard.press("c")
    await expect(page.getByTestId("target-bar")).toBeVisible()
    await page.keyboard.press("Escape")
    await expect(page.getByTestId("target-bar")).toHaveCount(0)

    const from = (await commitRow(page, "Bump Go version").boundingBox())!
    const to = (await commitRow(page, "Document the health handler").boundingBox())!
    await page.mouse.move(from.x + 20, from.y + from.height / 2)
    await page.mouse.down()
    await page.mouse.move(from.x + 40, from.y + 8, { steps: 4 })
    await page.mouse.move(to.x + 40, to.y + to.height / 2, { steps: 6 })
    await expect(page.getByTestId("drop-hint")).toHaveText("Squash into this commit")
    await page.screenshot({ path: `${shots}/drag-squash.png` })
    await page.keyboard.press("Escape")
    await page.mouse.up()
  })

  test("r squashes into a commit with both messages in the composer", async ({ page }) => {
    await commitRow(page, "Bump Go version").click()
    await page.keyboard.press("r")
    await commitRow(page, "Document the health handler").hover()
    await expect(page.getByTestId("target-desc")).toHaveText(
      "Squash into commit “Document the health handler”: commit “Bump Go version”",
    )
    await settle(page)
    await page.screenshot({ path: `${shots}/target-squash.png` })
    await commitRow(page, "Document the health handler").click()
    const dialog = page.getByRole("dialog")
    await expect(dialog.locator("textarea")).toHaveValue("Document the health handler\n\nBump Go version")
    await settle(page)
    await page.screenshot({ path: `${shots}/squash-composer.png` })
    await dialog.locator("textarea").fill("Document the handler and bump Go")
    await dialog.getByRole("button", { name: "Squash" }).click()
    await expect(card(page, "fix-typo").getByTestId("commit-row")).toHaveCount(1)
    await expect(commitRow(page, "Document the handler and bump Go")).toHaveCount(1)
  })

  test("move from the context menu, picking the target from the list", async ({ page }) => {
    await commitRow(page, "Test token auth").click({ button: "right" })
    await page.getByRole("menu").getByText("Move…").click()
    await expect(page.getByTestId("target-bar")).toBeVisible()
    await page.getByTestId("target-bar").getByRole("button", { name: /Targets/ }).click()
    const list = page.getByRole("dialog")
    await expect(list.getByRole("option", { name: /empty/ })).toBeVisible()
    await settle(page)
    await page.screenshot({ path: `${shots}/target-list.png` })
    await list.getByRole("combobox").fill("empty")
    await list.getByRole("option", { name: /empty/ }).click()
    await expect(card(page, "empty").getByText("Test token auth")).toBeVisible()
    await expect(card(page, "auth").getByText("Test token auth")).toHaveCount(0)
  })

  test("p with b cherry-picks onto a new branch above the target", async ({ page }) => {
    await commitRow(page, "Bump Go version").click()
    await page.keyboard.press("p")
    await expect(page.getByTestId("target-bar")).toBeVisible()
    await page.keyboard.press("b")
    await card(page, "empty").locator("button").first().hover()
    await expect(page.getByTestId("target-tag")).toHaveText("New branch above empty")
    await expect(page.getByTestId("target-desc")).toHaveText(
      "Cherry-pick commit “Bump Go version” to a new branch above empty",
    )
    await settle(page)
    await page.screenshot({ path: `${shots}/target-pick-new-branch.png` })
    await card(page, "empty").locator("button").first().click()
    await expect(commitRow(page, "Bump Go version")).toHaveCount(2)
    await expect(card(page, "empty").getByTestId("commit-row")).toHaveCount(0)
  })

  test("enter rewords with the full message", async ({ page }) => {
    await commitRow(page, "Add users endpoint").click()
    await page.keyboard.press("Enter")
    const dialog = page.getByRole("dialog")
    await expect(dialog.locator("textarea")).toHaveValue("Add users endpoint")
    await dialog.locator("textarea").fill("Add the users endpoint\n\nLists every user.")
    await settle(page)
    await page.screenshot({ path: `${shots}/reword.png` })
    await page.keyboard.press("ControlOrMeta+Enter")
    await expect(commitRow(page, "Add the users endpoint")).toHaveCount(1)
    await commitRow(page, "Add the users endpoint").click()
    await page.keyboard.press("Enter")
    await expect(page.getByRole("dialog").locator("textarea")).toHaveValue(
      "Add the users endpoint\n\nLists every user.",
    )
    await page.keyboard.press("Escape")
  })

  test("n inserts an empty commit", async ({ page }) => {
    await card(page, "empty").locator("button").first().click()
    await page.keyboard.press("n")
    await expect(card(page, "empty").getByTestId("commit-row")).toHaveCount(1)
  })

  test("x discards after a confirm, and the toast undoes it", async ({ page }) => {
    await fileRow(page, "src/server.go").click()
    await page.keyboard.press("x")
    const confirm = page.getByRole("alertdialog")
    await expect(confirm.getByText("Discard file src/server.go?")).toBeVisible()
    await settle(page)
    await page.screenshot({ path: `${shots}/discard-confirm.png` })
    await confirm.getByRole("button", { name: "Discard" }).click()
    await expect(fileRow(page, "src/server.go")).toHaveCount(0)
    // The selection falls back to Unstaged instead of a diff of a file that is gone.
    await expect(page.getByText(/Could not find target/)).toHaveCount(0)
    // The toast's Undo, not the header's.
    const undo = page.locator("[data-sonner-toast]").getByRole("button", { name: "Undo" })
    await expect(undo).toBeVisible()
    await page.screenshot({ path: `${shots}/discard-toast.png` })
    await undo.click()
    await expect(fileRow(page, "src/server.go")).toHaveCount(1)
  })

  test("A absorbs after a confirm and says where the changes went", async ({ page }) => {
    await page.getByTestId("unstaged-panel").locator("button").first().click()
    await page.keyboard.press("A")
    const confirm = page.getByRole("alertdialog")
    await expect(confirm.getByText("Absorb all uncommitted changes?")).toBeVisible()
    await confirm.getByRole("button", { name: "Absorb" }).click()
    await expect(page.getByText("Absorbed all uncommitted changes")).toBeVisible()
    await expect(page.getByText(/Absorbed to commit/)).toBeVisible()
    await expect(page.getByText(/AGENT ACTION/)).toHaveCount(0)
    await expect(page.getByTestId("file-row")).toHaveCount(0)
    await settle(page)
    await page.screenshot({ path: `${shots}/absorb-result.png` })
  })
})

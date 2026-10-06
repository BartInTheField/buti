import { mkdirSync } from "node:fs"
import { expect, test, type Page } from "@playwright/test"

// Branch lifecycle and history against the real fixture. Every test that changes the
// workspace undoes its change, so the specs after this one see the fixture as built.

const shots = `${process.env.BUTI_SHOTS ?? "/tmp/buti-desktop-shots"}/branches`
mkdirSync(shots, { recursive: true })

test.skip(() => Boolean(process.env.BUTI_E2E_SKIP), process.env.BUTI_E2E_SKIP)

const card = (page: Page, name: string) => page.locator(`[data-testid="branch-card"][data-branch="${name}"]`)

/** selectBranch clicks a branch card's header, the way a user selects it. */
async function selectBranch(page: Page, name: string) {
  await card(page, name).locator("button").first().click()
}

/** settle waits out open/close animations so screenshots show the finished state. */
async function settle(page: Page) {
  await page.waitForFunction(() => document.getAnimations().every((a) => a.playState !== "running"))
}

async function shot(page: Page, name: string) {
  await settle(page)
  await page.screenshot({ path: `${shots}/${name}.png` })
}

/** undo presses u and waits for the toast naming what was undone. */
async function undo(page: Page) {
  await page.locator("body").press("u")
  await expect(page.getByText(/^Undid “/).last()).toBeVisible()
}

async function cardBoxes(page: Page) {
  return page.getByTestId("branch-card").evaluateAll((els) =>
    els.map((el) => {
      const r = el.getBoundingClientRect()
      return [r.x, r.y, r.width, r.height]
    }),
  )
}

test.beforeEach(async ({ page }) => {
  await page.goto("/")
  await expect(page.getByTestId("branch-card").first()).toBeVisible()
})

test("branch cards show push state badges and a ⋯ menu without layout shift", async ({ page }) => {
  const before = await cardBoxes(page)
  await expect(card(page, "auth").getByTestId("push-badge")).toHaveText("local")
  await expect(card(page, "fix-typo").getByTestId("push-badge")).toBeVisible()

  await card(page, "fix-typo").getByTestId("push-badge").hover()
  await expect(page.getByRole("tooltip")).toBeVisible()
  expect(await cardBoxes(page)).toEqual(before)
  await shot(page, "badge-tooltip")

  await card(page, "fix-typo").getByTestId("branch-menu").click()
  const menu = page.getByRole("menu")
  await expect(menu.getByRole("menuitem", { name: /Push branch/ })).toBeVisible()
  await expect(menu.getByRole("menuitem", { name: /Unapply stack/ })).toBeVisible()
  await expect(menu.getByRole("menuitem", { name: /Delete branch/ })).toBeVisible()
  // One rename: branches.ts owns it; commits.ts's enter only rewords commits.
  await expect(menu.getByRole("menuitem", { name: /Rename|Reword/ })).toHaveCount(1)
  expect(await cardBoxes(page)).toEqual(before)
  await shot(page, "branch-menu")
  await page.keyboard.press("Escape")
})

test("b creates a branch stacked on the selection, u undoes it", async ({ page }) => {
  await selectBranch(page, "fix-typo")
  await page.keyboard.press("b")
  const dialog = page.getByRole("dialog")
  await expect(dialog.getByText("New branch stacked on fix-typo")).toBeVisible()
  await dialog.getByRole("textbox").fill("e2e stacked")
  await shot(page, "new-branch-prompt")
  await dialog.getByRole("button", { name: "Create" }).click()

  await expect(card(page, "e2e-stacked")).toBeVisible()
  // Stacked: the new card sits in the same lane as fix-typo.
  const lane = page.getByTestId("lane").filter({ has: card(page, "fix-typo") })
  await expect(lane.locator('[data-branch="e2e-stacked"]')).toBeVisible()
  // On top of fix-typo: lanes list the top of the stack first.
  await expect(lane.getByTestId("branch-card").first()).toHaveAttribute("data-branch", "e2e-stacked")
  await shot(page, "new-branch-created")

  await undo(page)
  await expect(card(page, "e2e-stacked")).toHaveCount(0)
  // The selection moved off the branch that is gone, so the details pane has no error.
  await expect(page.getByRole("heading", { name: "Unstaged changes" })).toBeVisible()
  await shot(page, "undo-toast")
})

test("enter renames a branch", async ({ page }) => {
  await selectBranch(page, "empty")
  await page.keyboard.press("Enter")
  const dialog = page.getByRole("dialog")
  await expect(dialog.getByRole("textbox")).toHaveValue("empty")
  await dialog.getByRole("textbox").fill("renamed empty")
  await dialog.getByRole("button", { name: "Rename" }).click()
  await expect(card(page, "renamed-empty")).toBeVisible()
  await expect(card(page, "empty")).toHaveCount(0)
  await undo(page)
  await expect(card(page, "empty")).toBeVisible()
})

test("delete a branch from its ⋯ menu, after a confirm", async ({ page }) => {
  await card(page, "empty").getByTestId("branch-menu").click()
  await page.getByRole("menuitem", { name: /Delete branch/ }).click()
  const confirm = page.getByRole("alertdialog")
  await expect(confirm.getByText("Delete branch empty?")).toBeVisible()
  await shot(page, "delete-confirm")
  await confirm.getByRole("button", { name: "Delete" }).click()
  await expect(card(page, "empty")).toHaveCount(0)
  await undo(page)
  await expect(card(page, "empty")).toBeVisible()
})

test("a applies a branch from the picker, S unapplies it after a confirm", async ({ page }) => {
  await page.locator("body").press("a")
  const picker = page.getByRole("dialog")
  const option = picker.getByRole("option", { name: /old-experiment/ })
  await expect(option).toBeVisible()
  await shot(page, "apply-picker")
  await option.click()
  await expect(card(page, "old-experiment")).toBeVisible()
  await expect(card(page, "old-experiment")).toHaveClass(/ring-1/) // selected

  await page.keyboard.press("S")
  const confirm = page.getByRole("alertdialog")
  await expect(confirm.getByText(/Unapply the stack with old-experiment/)).toBeVisible()
  await shot(page, "unapply-confirm")
  await confirm.getByRole("button", { name: "Unapply" }).click()
  await expect(card(page, "old-experiment")).toHaveCount(0)
})

test("P on a pushed branch says there is nothing to push", async ({ page }) => {
  await selectBranch(page, "api")
  await page.keyboard.press("P")
  await expect(page.getByText("api has nothing to push")).toBeVisible()
})

test("N asks for a pull request title, description and draft", async ({ page }) => {
  await selectBranch(page, "auth")
  await page.keyboard.press("N")
  const dialog = page.getByTestId("pr-dialog")
  await expect(dialog.getByText("For auth.")).toBeVisible()
  await dialog.getByLabel("Title").fill("Add auth")
  await dialog.getByLabel("Description").fill("Sign in with a token.")
  await dialog.getByRole("checkbox").click()
  await expect(dialog.getByRole("checkbox")).toBeChecked()
  await shot(page, "pr-dialog")
  await dialog.getByRole("button", { name: "Cancel" }).click()
  await expect(dialog).toHaveCount(0)
})

test("H lists the operation history with restore", async ({ page }) => {
  await page.getByRole("button", { name: "Operation history" }).click()
  const sheet = page.getByTestId("oplog-sheet")
  const entry = sheet.getByTestId("oplog-entry").first()
  await expect(entry).toBeVisible()
  await entry.hover()
  await shot(page, "oplog-sheet")
  await entry.getByRole("button", { name: "Restore" }).click()
  const confirm = page.getByRole("alertdialog")
  await expect(confirm.getByText(/^Restore to before/)).toBeVisible()
  await shot(page, "oplog-restore-confirm")
  await confirm.getByRole("button", { name: "Cancel" }).click()
  await expect(confirm).toHaveCount(0)
  await page.keyboard.press("Escape")
  await expect(sheet).toHaveCount(0)

  await page.locator("body").press("H")
  await expect(page.getByTestId("oplog-sheet")).toBeVisible()
})

test("t goes to a branch; the header pulls upstream", async ({ page }) => {
  await page.locator("body").press("t")
  const picker = page.getByRole("dialog")
  await picker.getByRole("combobox").fill("fix")
  await shot(page, "goto-branch")
  await picker.getByRole("option", { name: /fix-typo/ }).click()
  await expect(card(page, "fix-typo")).toHaveClass(/ring-1/)

  const upstream = page.getByRole("button", { name: /upstream \+1/ })
  await upstream.hover()
  await expect(page.getByRole("tooltip")).toContainText("Pull")
  await shot(page, "upstream-button")
})

test("palette offers land and clean up", async ({ page }) => {
  await selectBranch(page, "auth")
  await page.locator("body").press("Control+p")
  const palette = page.getByRole("dialog")
  await palette.getByRole("combobox").fill("land")
  await expect(palette.getByRole("option", { name: /Land branch onto target/ })).toBeVisible()
  await palette.getByRole("combobox").fill("clean")
  await expect(palette.getByRole("option", { name: /Clean up empty branches/ })).toBeVisible()
  await page.keyboard.press("Escape")
})

test("dark mode: badges, ⋯ menu and history", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "dark" })
  await page.reload()
  await expect(page.getByTestId("branch-card").first()).toBeVisible()
  await card(page, "api").getByTestId("branch-menu").click()
  await expect(page.getByRole("menu")).toBeVisible()
  await shot(page, "dark-branch-menu")
  await page.keyboard.press("Escape")
  await expect(page.getByRole("menu")).toHaveCount(0)
  await page.locator("body").press("H")
  await expect(page.getByTestId("oplog-entry").first()).toBeVisible()
  await shot(page, "dark-oplog-sheet")
})

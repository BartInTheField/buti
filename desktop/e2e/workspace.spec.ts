import { mkdirSync } from "node:fs"
import { expect, test, type Locator, type Page } from "@playwright/test"

const shots = process.env.BUTI_SHOTS ?? "/tmp/buti-desktop-shots"
mkdirSync(shots, { recursive: true })

test.skip(() => Boolean(process.env.BUTI_E2E_SKIP), process.env.BUTI_E2E_SKIP)

type Box = { x: number; y: number; width: number; height: number }

/** boxes measures every branch card, commit row and lane, the layout the drop targets live in. */
async function boxes(page: Page): Promise<Box[]> {
  return page
    .locator('[data-testid="branch-card"], [data-testid="commit-row"], [data-testid="new-branch-lane"]')
    .evaluateAll((els) =>
      els.map((el) => {
        const r = el.getBoundingClientRect()
        return { x: r.x, y: r.y, width: r.width, height: r.height }
      }),
    )
}

async function center(l: Locator) {
  const b = (await l.boundingBox())!
  return { x: b.x + b.width / 2, y: b.y + b.height / 2, box: b }
}

/** hintsWhileStill samples the drop hint across a few frames with the pointer still. */
async function hintsWhileStill(page: Page, samples = 12): Promise<string[]> {
  const seen: string[] = []
  for (let i = 0; i < samples; i++) {
    await page.waitForTimeout(25)
    const hints = await page.getByTestId("drop-hint").allTextContents()
    seen.push(hints.join("|"))
  }
  return seen
}

/** settle waits out open/close animations so screenshots show the finished state. */
async function settle(page: Page) {
  await page.waitForFunction(() => document.getAnimations().every((a) => a.playState !== "running"))
}

async function startDrag(page: Page, file: Locator) {
  const from = await center(file)
  await page.mouse.move(from.x, from.y)
  await page.mouse.down()
  // Past the 6px activation distance, then a few steps so dnd-kit sees a real drag.
  await page.mouse.move(from.x + 20, from.y + 4, { steps: 4 })
}

test.beforeEach(async ({ page }) => {
  await page.goto("/")
  await expect(page.getByTestId("branch-card").first()).toBeVisible()
  await expect(page.getByTestId("file-row").first()).toBeVisible()
})

test("hovering a branch card or a commit while dragging a file never changes layout", async ({ page }) => {
  const before = await boxes(page)
  const card = page.locator('[data-testid="branch-card"][data-branch="fix-typo"]')
  const file = page.getByTestId("file-row").filter({ hasText: "README.md" })

  await startDrag(page, file)

  // Across the card: header, the gap between commits, and the bottom padding.
  const c = await center(card)
  const points = [
    { x: c.x, y: c.box.y + 12 },
    { x: c.x + 30, y: c.box.y + 20 },
    { x: c.x, y: c.box.y + c.box.height - 4 },
  ]
  for (const p of points) {
    await page.mouse.move(p.x, p.y, { steps: 5 })
    const seen = await hintsWhileStill(page)
    expect(new Set(seen).size, `hint flickered at ${JSON.stringify(p)}: ${seen}`).toBe(1)
    expect(seen[0]).toBe("Commit onto fix-typo")
    expect(await boxes(page)).toEqual(before)
  }
  await page.screenshot({ path: `${shots}/drag-over-branch.png` })

  // Over a commit inside that card: the innermost target wins, steadily.
  const commit = card.getByTestId("commit-row").first()
  const cc = await center(commit)
  for (const dx of [-60, 0, 60]) {
    await page.mouse.move(cc.x + dx, cc.y, { steps: 5 })
    const seen = await hintsWhileStill(page)
    expect(new Set(seen).size, `hint flickered over commit: ${seen}`).toBe(1)
    expect(seen[0]).toBe("Amend into this commit")
    expect(await boxes(page)).toEqual(before)
  }
  await page.screenshot({ path: `${shots}/drag-over-commit.png` })

  // Back and forth across the card/commit edge: one hint at a time, layout fixed.
  for (let i = 0; i < 4; i++) {
    await page.mouse.move(cc.x, cc.box.y - 3, { steps: 3 })
    await page.mouse.move(cc.x, cc.box.y + 4, { steps: 3 })
    expect((await page.getByTestId("drop-hint").count()) <= 1).toBe(true)
    expect(await boxes(page)).toEqual(before)
  }

  await page.keyboard.press("Escape")
  await page.mouse.up()
  await expect(page.getByTestId("drop-hint")).toHaveCount(0)
})

test("dropping a file on a branch asks for a commit message", async ({ page }) => {
  const card = page.locator('[data-testid="branch-card"][data-branch="fix-typo"]')
  await startDrag(page, page.getByTestId("file-row").filter({ hasText: "README.md" }))
  const c = await center(card)
  await page.mouse.move(c.x, c.box.y + 12, { steps: 5 })
  await page.mouse.up()
  const dialog = page.getByRole("dialog")
  await expect(dialog.getByText("Commit onto fix-typo.")).toBeVisible()
  await settle(page)
  await page.screenshot({ path: `${shots}/commit-dialog.png` })
  await dialog.getByRole("button", { name: "Cancel" }).click()
  await expect(dialog).toHaveCount(0)
})

test("palette, help, context menu and marks", async ({ page }) => {
  await page.keyboard.press("ControlOrMeta+k")
  const palette = page.getByRole("dialog")
  await expect(palette.getByRole("option", { name: /Reload/ })).toBeVisible()
  await settle(page)
  await page.screenshot({ path: `${shots}/palette.png` })
  await page.keyboard.press("Escape")
  await expect(page.getByRole("dialog")).toHaveCount(0)

  await page.keyboard.press("?")
  await expect(page.getByRole("dialog").getByRole("option", { name: /Amend all changes into this/ })).toBeVisible()
  await settle(page)
  await page.screenshot({ path: `${shots}/help.png` })
  await page.keyboard.press("Escape")
  await expect(page.getByRole("dialog")).toHaveCount(0)

  await page.getByTestId("commit-row").first().click({ button: "right" })
  const menu = page.getByRole("menu")
  await expect(menu.getByText("Move…")).toBeVisible()
  await expect(menu.getByText("Uncommit to Unstaged")).toBeVisible()
  await settle(page)
  await page.screenshot({ path: `${shots}/context-menu.png` })
  await page.keyboard.press("Escape")
  // A closing menu owns Escape until its exit animation ends.
  await expect(menu).toHaveCount(0)

  const files = page.getByTestId("file-row")
  await files.nth(0).click()
  await page.keyboard.press(" ")
  await files.nth(1).click({ modifiers: ["ControlOrMeta"] })
  await expect(page.getByTestId("marks")).toHaveText("2 files marked")
  await settle(page)
  await page.screenshot({ path: `${shots}/marks.png` })
  await page.keyboard.press("Escape")
  await expect(page.getByTestId("marks")).toHaveCount(0)
})

test("workspace overview", async ({ page }) => {
  await page.screenshot({ path: `${shots}/workspace.png` })
})

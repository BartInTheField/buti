import { defineConfig } from "@playwright/test"

// End-to-end tests of the React screen against a real `buti desktop --serve` on the
// test repository. Run with `npm run e2e`; screenshots go to $BUTI_SHOTS
// (default /tmp/buti-desktop-shots).
export default defineConfig({
  testDir: "./e2e",
  globalSetup: "./e2e/global-setup.ts",
  timeout: 60_000,
  workers: 1,
  use: {
    baseURL: "http://127.0.0.1:1421",
    viewport: { width: 1400, height: 900 },
  },
})

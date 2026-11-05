import { defineConfig, devices } from "@playwright/test";
export default defineConfig({
  testDir: "./tests",
  workers: 1,
  use: {
    baseURL: process.env.E2E_URL || "http://127.0.0.1:3000",
    trace: "retain-on-failure",
    channel: process.env.E2E_BROWSER_CHANNEL,
    launchOptions: { executablePath: process.env.E2E_BROWSER_EXECUTABLE },
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});

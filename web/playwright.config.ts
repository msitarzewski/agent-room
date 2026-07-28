import { defineConfig, devices } from "@playwright/test";

const realDaemon = process.env.REAL_DAEMON === "1";
const nativeDarwin = process.platform === "darwin" && !process.env.CI;

if (realDaemon) {
  const missing = [
    ["AGENT_ROOM_E2E_BASE_URL", process.env.AGENT_ROOM_E2E_BASE_URL],
    ["AGENT_ROOM_E2E_USERNAME", process.env.AGENT_ROOM_E2E_USERNAME],
    ["AGENT_ROOM_E2E_PASSWORD", process.env.AGENT_ROOM_E2E_PASSWORD],
  ].filter(([, value]) => !value).map(([name]) => name);

  if (missing.length > 0) {
    throw new Error(`REAL_DAEMON=1 requires: ${missing.join(", ")}`);
  }
}

export default defineConfig({
  testDir: "./tests/browser",
  fullyParallel: false,
  forbidOnly: true,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 2 : 3,
  reporter: [["list"], ["html", { open: "never" }]],
  use: {
    baseURL: process.env.AGENT_ROOM_E2E_BASE_URL ?? "http://127.0.0.1:4173",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: realDaemon
    ? [{
        name: "real-daemon-chromium",
        testMatch: /real-daemon\.spec\.ts/,
        use: { ...devices["Desktop Chrome"] },
      }]
    : [
        {
          name: "chromium",
          testIgnore: /real-daemon\.spec\.ts/,
          use: { ...devices["Desktop Chrome"] },
        },
        {
          name: "firefox",
          testIgnore: /real-daemon\.spec\.ts/,
          use: { ...devices["Desktop Firefox"] },
        },
        {
          name: "webkit",
          testIgnore: /real-daemon\.spec\.ts/,
          use: { ...devices["Desktop Safari"] },
        },
      ].filter((project) => !nativeDarwin || project.name !== "firefox"),
  webServer: realDaemon ? undefined : {
    command: "npm run dev",
    url: "http://127.0.0.1:4173",
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
});

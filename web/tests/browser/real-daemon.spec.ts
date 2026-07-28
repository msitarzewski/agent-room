import { expect, test } from "@playwright/test";

const username = process.env.AGENT_ROOM_E2E_USERNAME!;
const password = process.env.AGENT_ROOM_E2E_PASSWORD!;

test.describe("real daemon", () => {
  test("authenticates through OIDC and supervises the live project", async ({ page }) => {
    const serverFailures: string[] = [];
    const consoleErrors: Array<{ authenticated: boolean; text: string }> = [];
    const postAuthenticationUnauthorized: string[] = [];
    let authenticated = false;
    let initialSessionUnauthorized = 0;
    page.on("response", (response) => {
      if (response.status() >= 500) {
        serverFailures.push(`${response.status()} ${response.url()}`);
      }
      if (response.status() === 401) {
        const path = new URL(response.url()).pathname;
        if (!authenticated && path === "/api/v1/auth/session") {
          initialSessionUnauthorized += 1;
        } else if (authenticated) {
          postAuthenticationUnauthorized.push(response.url());
        }
      }
    });
    page.on("console", (message) => {
      if (message.type() === "error") consoleErrors.push({ authenticated, text: message.text() });
    });

    const unauthorized = await page.request.get("/api/v1/tasks?project_id=unauthorized");
    expect(unauthorized.status()).toBe(401);
    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Sign in" })).toBeVisible();
    await page.getByRole("button", { name: "Continue securely" }).click();
    await expect(page).not.toHaveURL(/\/login$/);
    await page.locator('input[name="login"], input[name="username"], input[type="email"]').first().fill(username);
    await page.locator('input[name="password"], input[type="password"]').first().fill(password);
    await page.getByRole("button", { name: /sign in|login/i }).click();
    const consent = page.getByRole("button", { name: /grant access/i });
    if (await consent.isVisible({ timeout: 2_000 }).catch(() => false)) {
      await consent.click();
    }

    await expect(page.getByRole("heading", { name: "What needs you now" })).toBeVisible();
    authenticated = true;
    await expect(page.getByLabel("Project")).not.toHaveValue("");

    for (const [link, heading] of [
      ["Workers & runs", "Workers & runs"],
      ["Tasks & review", "Tasks & review"],
      ["Approvals", "Approvals"],
      ["Timeline", "Timeline"],
      ["Evidence", "Evidence & artifacts"],
      ["Chat", "Chat"],
      ["Costs & budgets", "Costs & budgets"],
      ["Health", "Health & connectivity"],
      ["Sessions", "Active sessions"],
    ] as const) {
      await page.getByRole("link", { name: new RegExp(link) }).click();
      await expect(page.getByRole("heading", { name: heading })).toBeVisible();
    }

    await page.getByRole("link", { name: /Chat/ }).click();
    const text = `Playwright live verification ${Date.now()}`;
    await page.getByLabel("Message this project").fill(text);
    const mutation = page.waitForResponse(
      (response) => response.url().includes("/api/v1/chat/messages") && response.request().method() === "POST",
    );
    await page.getByRole("button", { name: "Send message" }).click();
    expect((await mutation).status()).toBe(201);
    await expect(page.getByText("Message posted.")).toBeVisible();

    await page.getByRole("link", { name: /Attention/ }).click();
    await expect(page.getByRole("heading", { name: "Since you were away" })).toBeVisible();
    const acknowledge = page.waitForResponse(
      (response) => response.url().includes("/api/v1/brief/acknowledge") && response.request().method() === "POST",
    );
    await page.getByRole("button", { name: "Mark changes reviewed" }).click();
    expect((await acknowledge).status()).toBe(200);
    await expect(page.getByText(/Changes through event \d+ were marked reviewed/)).toBeVisible();

    await page.getByRole("link", { name: /Evidence/ }).click();
    await expect(
      page.getByText("No artifacts indexed").or(page.getByRole("link", { name: "Download verified artifact" }).first()),
    ).toBeVisible();
    await page.getByRole("link", { name: /Workers & runs/ }).click();
    await expect(
      page.getByText("No runs recorded").or(page.locator(".detail-card").first()),
    ).toBeVisible();

    expect(serverFailures).toEqual([]);
    expect(postAuthenticationUnauthorized).toEqual([]);
    expect(initialSessionUnauthorized).toBe(1);
    const expectedInitialConsoleErrors = consoleErrors.filter(
      (entry) => !entry.authenticated && /failed to load resource/i.test(entry.text) && /\b401\b/.test(entry.text),
    );
    expect(expectedInitialConsoleErrors).toHaveLength(1);
    expect(consoleErrors.filter((entry) => !expectedInitialConsoleErrors.includes(entry))).toEqual([]);
  });
});

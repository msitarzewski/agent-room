import { expect, test, type Page, type Route } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

const base = { version: 1, project_id: "project_alpha", created_at: "2026-07-27T11:00:00Z", updated_at: "2026-07-27T12:00:00Z" };
const source = { system: "codex", external_id: "native_1" };
const data: Record<string, unknown[]> = {
  "/api/v1/projects": [{
    id: "project_alpha", name: "Agent Room", version: 1,
    updated_at: "2026-07-27T12:00:00Z", capabilities: ["*"],
  }],
  "/api/v1/attention": [{
    ...base, id: "attention_1", kind: "review", title: "Review authentication evidence",
    detail: "Builder submitted a claim that requires review.", severity: "high", status: "open",
    owner_id: "human_operator", resource_type: "task", resource_id: "task_1",
  }],
  "/api/v1/agents": [{
    ...base, id: "agent_builder", display_name: "Builder", kind: "agent", status: "active",
    capabilities: [], source,
  }],
  "/api/v1/runs": [{
    ...base, id: "run_1", agent_id: "agent_builder", task_id: "task_1",
    summary: "Build secure session flow", status: "working",
    capabilities: ["run:pause", "run:message", "run:redirect", "run:cancel"], source,
  }],
  "/api/v1/tasks": [{
    ...base, id: "task_1", title: "Build secure session flow", description: "Implement OIDC.",
    owner_id: "agent_builder", status: "review", priority: "high", dependencies: [],
    review_state: "requested", approval_state: "pending", source,
  }],
  "/api/v1/events": [{
    id: "event_1", cursor: 42, project_id: "project_alpha", type: "review.requested",
    subject_type: "task", subject_id: "task_1", actor_id: "agent_builder",
    occurred_at: "2026-07-27T12:00:00Z", correlation_id: "correlation_1", schema_version: 1, payload: {},
  }],
  "/api/v1/evidence": [{
    ...base, id: "evidence_1", task_id: "task_1", run_id: "run_1", kind: "test_result",
    summary: "Authentication test report", digest: "sha256:abc123", source,
  }],
  "/api/v1/artifacts": [{
    ...base, id: "artifact_1", name: "test-report.json", media_type: "application/json",
    digest: "sha256:abc123", uri: "/api/v1/artifacts/artifact_1/content", source,
  }],
  "/api/v1/chat/messages": [{
    ...base, id: "message_1", author_id: "agent_pip", role: "assistant",
    body: "I found one review that needs your attention.",
  }],
  "/api/v1/budgets": [{
    ...base, id: "budget_1", name: "Daily token budget", scope_type: "project",
    scope_id: "project_alpha", token_limit: 100000, token_used: 38000,
  }],
  "/api/v1/sessions": [{
    ...base, id: "session_1", agent_id: "agent_pip", conversation_ref: "Agent Room release",
    external_ref: "hermes_session_1", status: "active", source: { system: "hermes", external_id: "hermes_session_1" },
  }],
  "/api/v1/claims": [{
    ...base, id: "claim_1", task_id: "task_1", agent_id: "agent_builder", status: "pending",
  }],
  "/api/v1/approvals": [{
    ...base, id: "approval_cancel_run_1", kind: "run_action", status: "approved",
    resource_type: "run", resource_id: "run_1", requested_by: "human_operator",
    reason: "Stop a run that is no longer safe to continue.", command_digest: "sha256:abc123",
    command_version: 1, expected_target_version: 1,
    expires_at: "2099-07-27T20:00:00Z", context: { action: "cancel" },
  }],
};

async function fulfillApi(route: Route, empty = false) {
  const url = new URL(route.request().url());
  if (url.pathname === "/api/v1/auth/session") {
    return route.fulfill({ json: {
      user: { id: "human_operator", username: "operator", display_name: "Test Operator", capabilities: ["*"] },
      expires_at: "2026-07-27T20:00:00Z", csrf_token: "browser-csrf",
    } });
  }
  if (route.request().method() === "POST") {
    return route.fulfill({ json: { id: "updated", version: 2, project_id: "project_alpha", updated_at: new Date().toISOString() } });
  }
  if (url.pathname === "/api/v1/health/components") {
    return route.fulfill({ json: {
      schema: { status: "ok", migrations: 1 },
      event_outbox: { status: "ok", pending: 0, oldest_seconds: 0 },
      control_outbox: { status: "ok", pending: 0 },
      adapters: { status: "ok", last_seen: { codex: "2026-07-27T12:00:00Z" } },
      artifacts: { status: "ok" },
      oidc: { status: "ok", configured: true },
      realtime: { status: "ok" },
      checked_at: "2026-07-27T12:00:00Z",
    } });
  }
  if (url.pathname === "/api/v1/brief") {
    return route.fulfill({ json: {
      project_id: "project_alpha", from_cursor: 40, through_cursor: 42, reviewed_cursor: 40,
      event_counts: { "review.requested": 1 }, events: data["/api/v1/events"],
      open_attention: data["/api/v1/attention"], pending_approvals: [],
      recommended_actions: ["Triage open attention items"], generated_at: "2026-07-27T12:00:00Z",
    } });
  }
  return route.fulfill({ json: { items: empty ? [] : (data[url.pathname] ?? []), next_cursor: null } });
}

async function boot(page: Page, empty = false) {
  await page.routeWebSocket(/\/api\/v1\/stream/, () => {
    // The mocked socket intentionally remains server-push-only and quiet.
  });
  await page.route("**/api/v1/**", (route) => fulfillApi(route, empty));
  await page.addInitScript(() => localStorage.setItem("agent-room:project", "project_alpha"));
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "What needs you now" })).toBeVisible();
}

test("supervises work across every primary view", async ({ page }) => {
  await boot(page);
  await expect(page.getByText("Review authentication evidence")).toBeVisible();
  const views = [
    ["Workers & runs", "Build secure session flow"],
    ["Tasks & review", "Build secure session flow"],
    ["Approvals", "cancel · run_1"],
    ["Timeline", "task · task_1"],
    ["Evidence", "Authentication test report"],
    ["Chat", "Pip"],
    ["Costs & budgets", "Daily token budget"],
    ["Health", "PostgreSQL"],
    ["Sessions", "Agent Room release"],
  ] as const;
  for (const [link, content] of views) {
    await page.getByRole("link", { name: new RegExp(link) }).click();
    await expect(page.getByText(content, { exact: false }).first()).toBeVisible();
  }
});

test("executes only advertised run controls and preserves an explicit cancel confirmation", async ({ page }) => {
  await boot(page);
  await page.getByRole("link", { name: /Workers & runs/ }).click();
  await expect(page.getByRole("button", { name: "Pause" })).toBeVisible();
  await page.getByRole("button", { name: "Cancel run" }).click();
  await expect(page.getByText("Cancel Build secure session flow?")).toBeVisible();
  const requestPromise = page.waitForRequest((request) => request.url().includes("/runs/run_1/actions"));
  await page.getByRole("button", { name: "Submit approved cancellation" }).click();
  const request = await requestPromise;
  expect(request.headers()["x-csrf-token"]).toBe("browser-csrf");
  expect(request.headers()["if-match"]).toBe("1");
  expect(request.postDataJSON()).toMatchObject({ action: "cancel", approval_id: "approval_cancel_run_1" });
  await expect(page.getByText("cancel command accepted for Build secure session flow.")).toBeVisible();
});

test("shows honest empty state without sample production data", async ({ page }) => {
  await boot(page, true);
  await expect(page.getByText("The room is quiet")).toBeVisible();
  await expect(page.getByText("Review authentication evidence")).toHaveCount(0);
});

test("keyboard users can bypass navigation", async ({ page }) => {
  await boot(page);
  const skip = page.getByRole("link", { name: "Skip to content" });
  await skip.focus();
  await expect(skip).toBeFocused();
  await skip.press("Enter");
  await expect(page.locator("#main-content")).toBeFocused();
});

for (const viewport of [
  { width: 1440, height: 1000, label: "wide desktop" },
  { width: 1024, height: 768, label: "compact desktop" },
]) {
  test(`is accessible and does not overflow at ${viewport.label}`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await boot(page);
    const dimensions = await page.evaluate(() => ({
      viewport: document.documentElement.clientWidth,
      document: document.documentElement.scrollWidth,
    }));
    expect(dimensions.document).toBeLessThanOrEqual(dimensions.viewport);
    await expect(page.locator(".sidebar")).toBeVisible();
    await expect(page.locator("#main-content")).toBeVisible();
    const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21aa"]).analyze();
    expect(results.violations).toEqual([]);
    await page.screenshot({
      path: testInfo.outputPath(`${viewport.width}x${viewport.height}.png`),
      fullPage: true,
    });
  });
}

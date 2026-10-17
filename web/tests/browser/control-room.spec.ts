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
  if (url.pathname === "/api/v1/overview") {
    return route.fulfill({ json: { project_id: "project_alpha", counts: empty ? {} : { task: 2, attention: 1 }, event_cursor: 42, generated_at: new Date().toISOString() } });
  }
  if (url.pathname === "/api/v1/workspace") {
    const resources = Object.fromEntries(["tasks", "agents", "runs", "attention", "evidence", "artifacts", "claims", "approvals"].map((kind) => [kind, empty ? [] : data[`/api/v1/${kind}`] ?? []]));
    if (!empty) {
      resources.tasks = [
        { ...(data["/api/v1/tasks"]?.[0] as Record<string, unknown>), dependencies: ["task_dependency"], source_status: "completed" },
        { ...base, id: "task_dependency", title: "Validate identity boundary", status: "blocked", priority: "normal", dependencies: [], blocked_reason: "Waiting for provider evidence", source },
      ];
      resources.artifacts = (resources.artifacts ?? []).map((artifact) => ({ ...(artifact as Record<string, unknown>), task_id: "task_1" }));
    }
    return route.fulfill({ json: { project_id: "project_alpha", cursor: 42, generated_at: new Date().toISOString(), resources, sources: empty ? [] : [{ system: "codex", last_seen: new Date().toISOString() }] } });
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
  return route.fulfill({ json: { items: empty && url.pathname !== "/api/v1/projects" ? [] : (data[url.pathname] ?? []), next_cursor: null } });
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

async function bootThreshold(page: Page) {
  await page.route("**/api/v1/auth/session", (route) =>
    route.fulfill({ status: 401, json: { error: "authentication required" } }));
  await page.goto("/");
  await expect(page.getByRole("heading", {
    name: "The work continues. The room remembers.",
  })).toBeVisible();
}

for (const viewport of [
  { width: 1440, height: 900, label: "threshold desktop" },
  { width: 768, height: 1024, label: "threshold tablet" },
  { width: 390, height: 844, label: "threshold mobile" },
]) {
  test(`presents an accessible, private ${viewport.label}`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await bootThreshold(page);
    await expect(page.getByRole("button", { name: "Enter quietly" })).toBeVisible();
    await expect(page.getByText("If you are expected, the door already knows.")).toBeVisible();
    await expect(page.locator("body")).not.toContainText(
      /identity provider|network membership|authorization code/i,
    );
    const dimensions = await page.evaluate(() => ({
      viewport: document.documentElement.clientWidth,
      document: document.documentElement.scrollWidth,
    }));
    expect(dimensions.document).toBeLessThanOrEqual(dimensions.viewport);
    const results = await new AxeBuilder({ page })
      .withTags(["wcag2a", "wcag2aa", "wcag21aa"])
      .analyze();
    expect(results.violations).toEqual([]);
    await page.screenshot({
      path: testInfo.outputPath(`${viewport.width}x${viewport.height}.png`),
      fullPage: true,
    });
  });
}

test("threshold respects reduced motion and begins the OIDC handoff", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await bootThreshold(page);
  const motion = await page.locator(".threshold-orbit").first().evaluate((element) => {
    const style = getComputedStyle(element);
    return {
      durationSeconds: Number.parseFloat(style.animationDuration),
      iterations: style.animationIterationCount,
    };
  });
  expect(motion.durationSeconds).toBeLessThanOrEqual(0.00001);
  expect(motion.iterations).toBe("1");
  const login = page.waitForRequest((request) =>
    new URL(request.url()).pathname === "/api/v1/auth/login");
  await page.getByRole("button", { name: "Enter quietly" }).click();
  expect(new URL((await login).url()).searchParams.get("return_to")).toBe("/");
});

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

test("keeps 99 project contexts calm, exposes the actual source blocker and restores grouped focus", async ({ page }, testInfo) => {
  await boot(page);
  const now = new Date().toISOString();
  const projects = Array.from({ length: 99 }, (_, index) => ({ id: index === 0 ? "project_alpha" : `project_${String(index).padStart(3, "0")}`, name: `Project ${String(index).padStart(3, "0")}`, version: 1, updated_at: now, capabilities: ["*"] }));
  await page.route("**/api/v1/projects", (route) => route.fulfill({ json: { items: projects, next_cursor: null } }));
  await page.route("**/api/v1/overview?*", (route) => {
    const id = new URL(route.request().url()).searchParams.get("project_id");
    const blocked = id === "project_alpha";
    const working = id === "project_001";
    const partial = id === "project_002";
    return route.fulfill({ json: { project_id: id, event_cursor: 42, generated_at: now, counts: { task: blocked ? 1 : 0 }, sources: [{ system: "sentry", last_seen: now, last_collected_at: now, last_successful_collection_at: now, scan_status: partial ? "partial" : "complete" }], coordination: {
      complete: true, truncated: false, counts: { running_work: working ? 1 : 0, agent_presence: working ? 1 : 0, unfinished_tasks: 0, blocked_tasks: blocked ? 1 : 0, canonical_blocked_tasks: 0, source_blocked_tasks: blocked ? 1 : 0, requests: 0, reviews: 0, human_escalations: 0, human_approvals: 0 },
      items: blocked ? [{ kind: "blocker", resource_type: "task", id: "task_1", task_id: "task_1", title: "Recorded source dependency wait", status: "source:Blocked", detail: "Waiting for the provider's upstream evidence.", source_system: "sentry", updated_at: now, human_escalation: false }] : working ? [{ kind: "run", resource_type: "run", id: "actual_run", title: "Validate provider evidence", owner_id: "agent_builder", owner_name: "Builder", status: "running", updated_at: now, human_escalation: false, source_system: "codex" }] : [],
    } } });
  });
  await page.route("**/api/v1/workspace?*", (route) => {
    const id = new URL(route.request().url()).searchParams.get("project_id");
    return route.fulfill({ json: { project_id: id, cursor: 42, generated_at: now, resources: { tasks: [], agents: [], runs: [], attention: [], evidence: [], artifacts: [], claims: [], approvals: [], requests: [] }, sources: [] } });
  });
  await page.goto("/workspace");
  await expect(page.getByRole("button", { name: /Quiet.*96 projects/ })).toBeVisible();
  const coordination = page.getByRole("region", { name: "Coordination 2" });
  await expect(coordination.getByText("Builder", { exact: true })).toBeVisible();
  await expect(coordination.getByText("Validate provider evidence", { exact: true })).toBeVisible();
  await expect(coordination.getByText("Waiting for the provider's upstream evidence.")).toBeVisible();
  await expect(page.getByRole("region", { name: "Human escalations 0" })).toContainText("No explicit human escalation");
  const directory = page.getByRole("region", { name: "Your project directory" });
  await expect(directory.getByRole("listitem")).toHaveCount(0);
  await expect(page.locator(".workspace-scene")).toHaveAttribute("data-renderer", /^(WebGPU|WebGL 2|Accessible workspace)$/, { timeout: 15_000 });
  await page.locator(".observatory-world").screenshot({ path: testInfo.outputPath("coordination-grouped.png") });
  await page.getByRole("button", { name: /Quiet.*96 projects/ }).click();
  await expect(directory.getByRole("listitem")).toHaveCount(12);
  await page.getByRole("button", { name: "Next projects" }).click();
  await expect(page.getByText("Page 2 of 8")).toBeVisible();
  await directory.getByRole("button", { name: /Project 015/ }).click();
  await expect(page).toHaveURL(/project=project_015/);
  await page.getByRole("button", { name: "Portfolio", exact: true }).click();
  await expect(page.getByText("Page 2 of 8")).toBeVisible();
  await expect(directory.getByRole("listitem")).toHaveCount(12);
  await page.locator(".observatory-world").screenshot({ path: testInfo.outputPath("coordination-unfolded.png") });
  expect((await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21aa"]).analyze()).violations).toEqual([]);
});

test("inspects a real coordination request and confirms a versioned idempotent response", async ({ page }) => {
  await boot(page);
  let responded = false;
  const request = { ...base, id: "request_context", version: 3, kind: "context", title: "Need provider context", body: "Please review the redacted provider evidence.", owner_id: "agent_builder", owner_type: "agent", recipient: { type: "human", id: "human_operator" }, escalation_reason: "Only the owner can supply this context.", evidence_ids: [], status: "open" };
  await page.route("**/api/v1/workspace?*", (route) => route.fulfill({ json: { project_id: "project_alpha", cursor: 42, generated_at: new Date().toISOString(), resources: { tasks: [], agents: data["/api/v1/agents"], runs: [], attention: [], evidence: [], artifacts: [], claims: [], approvals: [], requests: [{ ...request, status: responded ? "responded" : "open", version: responded ? 4 : 3 }] }, sources: [] } }));
  await page.route("**/api/v1/requests/request_context/respond?*", async (route) => {
    expect(route.request().headers()["if-match"]).toBe("3");
    expect(route.request().headers()["idempotency-key"]).toBeTruthy();
    expect(route.request().headers()["x-csrf-token"]).toBe("browser-csrf");
    expect(route.request().postDataJSON()).toEqual({ body: "Reviewed; the provider context is verified." });
    responded = true;
    return route.fulfill({ json: { ...request, status: "responded", version: 4 } });
  });
  await page.goto("/workspace?project=project_alpha");
  await page.getByRole("button", { name: /Need provider context · context · open/ }).click();
  await expect(page.getByRole("article", { name: "Request details" })).toContainText("Builder (agent)");
  await expect(page.getByText("Escalation reason: Only the owner can supply this context.")).toBeVisible();
  await page.getByRole("textbox", { name: "Response or closure note" }).fill("Reviewed; the provider context is verified.");
  await page.getByRole("button", { name: "Review response" }).click();
  expect(responded).toBe(false);
  await expect(page.getByRole("alertdialog")).toContainText("version 3");
  await page.getByRole("button", { name: "Confirm coordination decision" }).click();
  await expect(page.getByText("Coordination decision recorded.")).toBeVisible();
  expect(responded).toBe(true);
  await expect(page.getByRole("button", { name: /Need provider context · context · responded/ })).toBeVisible();
});

for (const viewport of [
  { width: 1440, height: 1000, font: "100%" },
  { width: 760, height: 640, font: "100%" },
  { width: 390, height: 844, font: "100%" },
  { width: 390, height: 844, font: "200%" },
]) {
test(`keeps necessary workspace information separate at ${viewport.width}px and ${viewport.font} text`, async ({ page }, testInfo) => {
  await boot(page);
  const longIdentity = `Participant_${"LongNativeIdentity".repeat(16)}`;
  const longText = `${"Evidence and context remain available while coordinating this task. ".repeat(24)} End of complete evidence.`;
  const request = { ...base, id: "request_dense", kind: "context", title: "Detailed handoff context", body: longText, owner_id: "agent_builder", owner_type: "agent", recipient: { type: "human", id: "human_operator" }, escalation_reason: "The operator holds the missing context.", task_id: "task_1", evidence_ids: ["evidence_1"], status: "open" };
  const summaryItems = [
    { kind: "blocker", resource_type: "task", id: "task_1", title: longIdentity, detail: longText, status: "blocked", updated_at: base.updated_at, owner_name: longIdentity, human_escalation: false },
    { kind: "request", resource_type: "coordination_request", id: request.id, title: request.title, detail: request.escalation_reason, status: "open", updated_at: base.updated_at, owner_name: longIdentity, recipient_name: "Test Operator", recipient: request.recipient, human_escalation: true },
  ];
  await page.route("**/api/v1/overview?*", (route) => route.fulfill({ json: { project_id: "project_alpha", counts: { task: 1, attention: 1 }, event_cursor: 42, generated_at: new Date().toISOString(), sources: [], coordination: { complete: true, truncated: false, counts: { running_work: 0, agent_presence: 0, unfinished_tasks: 1, blocked_tasks: 1, canonical_blocked_tasks: 1, source_blocked_tasks: 0, requests: 1, reviews: 0, human_escalations: 1, human_approvals: 0 }, items: summaryItems } } }));
  await page.route("**/api/v1/workspace?*", (route) => route.fulfill({ json: { project_id: "project_alpha", cursor: 42, generated_at: new Date().toISOString(), resources: {
    tasks: [{ ...(data["/api/v1/tasks"]![0] as Record<string, unknown>), title: longIdentity, description: longText, dependencies: ["task_1", "unavailable_dependency"] }],
    agents: data["/api/v1/agents"], runs: data["/api/v1/runs"], attention: data["/api/v1/attention"],
    evidence: [{ ...(data["/api/v1/evidence"]![0] as Record<string, unknown>), summary: longText }], artifacts: [], claims: [], approvals: [], requests: [request],
  }, sources: [] } }));

    await page.setViewportSize(viewport);
    await page.goto("/workspace?project=project_alpha&task=task_1");
    await page.evaluate((font) => { document.documentElement.style.fontSize = font; }, viewport.font);
    await page.getByText("Connection details", { exact: true }).click();
    await expect(page.locator(".stream-health__body")).toBeVisible();
    const [header, content] = await Promise.all([page.locator(".topbar").boundingBox(), page.locator("#main-content").boundingBox()]);
    expect(header!.y + header!.height).toBeLessThanOrEqual(content!.y + 1);
    const notice = page.locator(".workspace-scene__message");
    await expect(notice).toContainText("Dependency details unavailable: 1");
    await expect(notice).toContainText("dependency cycle: 1");
    await expect(page.locator(".workspace-scene")).toHaveAttribute("data-renderer", /^(WebGPU|WebGL 2|Accessible workspace)$/, { timeout: 15_000 });
    const graphical = await page.locator(".workspace-scene").getAttribute("data-renderer") !== "Accessible workspace";
    const surfaces = graphical ? ["toolbar", "message", "canvas", "hint"] : ["toolbar", "message", "fallback"];
    const boxes = await Promise.all(surfaces.map((name) => page.locator(`.workspace-scene__${name}`).boundingBox()));
    for (const box of boxes) expect(box).not.toBeNull();
    for (let index = 0; index < boxes.length - 1; index++) expect(boxes[index]!.y + boxes[index]!.height).toBeLessThanOrEqual(boxes[index + 1]!.y + 1);
    expect(boxes[2]!.height).toBeGreaterThanOrEqual(300);
    if (graphical) {
      const recenter = page.getByRole("button", { name: "Recenter view" });
      await recenter.scrollIntoViewIfNeeded();
      expect(await recenter.evaluate((element) => {
        const box = element.getBoundingClientRect();
        const hit = document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2);
        return hit === element || hit !== null && element.contains(hit);
      })).toBe(true);
    } else {
      await expect(page.locator(".workspace-scene__fallback")).toContainText("Your data and evidence remain available.");
      await expect(page.getByRole("button", { name: "Recenter view" })).toHaveCount(0);
    }
    for (const selectors of [[".observatory-coordination > section:first-child", ".observatory-escalations"], [".observatory-world", ".observatory-inspector"]]) {
      const [a, b] = await Promise.all(selectors.map((selector) => page.locator(selector).boundingBox()));
      expect(a && b && (a.x + a.width <= b.x + 1 || b.x + b.width <= a.x + 1 || a.y + a.height <= b.y + 1 || b.y + b.height <= a.y + 1)).toBe(true);
    }
    const overflow = await page.evaluate(() => ({ document: document.documentElement.scrollWidth > document.documentElement.clientWidth, cards: Array.from(document.querySelectorAll<HTMLElement>(".observatory-coordination button, .observatory-inspector")).some((element) => element.scrollWidth > element.clientWidth + 1) }));
    expect(overflow).toEqual({ document: false, cards: false });
    const inspector = page.getByRole("complementary", { name: "Workspace inspector" });
    await expect(inspector).toContainText("End of complete evidence.");
    await inspector.getByRole("button", { name: "Detailed handoff context · context · open" }).click();
    await expect(page.getByRole("article", { name: "Request details" })).toContainText(longText);
    await page.getByRole("textbox", { name: "Response or closure note" }).fill("The complete context is visible and ready for review.");
    const trigger = page.getByRole("button", { name: "Review response" });
    await trigger.click();
    await expect(page.getByRole("alertdialog")).toBeFocused();
    // Enlarged context can require scrolling; the decision must remain reachable and unobscured.
    const confirm = page.getByRole("button", { name: "Confirm coordination decision" });
    await confirm.scrollIntoViewIfNeeded();
    await expect(confirm).toBeInViewport();
    expect(await confirm.evaluate((element) => {
      const box = element.getBoundingClientRect();
      const hit = document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2);
      return hit === element || hit !== null && element.contains(hit);
    })).toBe(true);
    await page.getByRole("button", { name: "Keep reviewing request" }).click();
    await expect(trigger).toBeFocused();
    if (viewport.font === "200%") await page.locator(".observatory-world").screenshot({ path: testInfo.outputPath("necessary-information-enlarged.png") });
});
}

test("unfolds the observatory into task evidence and a real dependency", async ({ page }, testInfo) => {
  await boot(page);
  await page.getByRole("link", { name: "Observatory" }).click();
  await expect(page.getByRole("heading", { name: "Your observatory" })).toBeVisible();
  const directory = page.getByRole("region", { name: "Your project directory" });
  await directory.getByRole("button", { name: /Agent Room/ }).click();
  await expect(page.getByRole("heading", { name: "Work in this project" })).toBeVisible();
  await page.getByRole("button", { name: /Build secure session flow.*dependencies/ }).click();
  const inspector = page.getByRole("complementary", { name: "Workspace inspector" });
  await expect(inspector.getByRole("heading", { name: "Build secure session flow" })).toBeVisible();
  await expect(inspector.getByText("Authentication test report")).toBeVisible();
  await expect(inspector.getByText("Source reports: completed. Supervised state: review.")).toBeVisible();
  await expect(inspector.getByRole("link", { name: /test-report.json/ })).toHaveAttribute("href", /project_id=project_alpha/);
  await inspector.getByRole("button", { name: "Validate identity boundary" }).click();
  await expect(inspector.getByText("Waiting for provider evidence")).toBeVisible();
  await expect(page).toHaveURL(/project=project_alpha.*task=task_dependency/);
  await expect(page.getByRole("heading", { name: /Attention 1/ })).toBeVisible();
  await expect(page.locator(".workspace-scene")).toHaveAttribute("data-renderer", /^(WebGPU|WebGL 2|Accessible workspace)$/, { timeout: 15_000 });
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: testInfo.outputPath("observatory-project.png"), fullPage: true });
  await page.setViewportSize({ width: 390, height: 844 });
  const dimensions = await page.evaluate(() => ({ viewport: document.documentElement.clientWidth, document: document.documentElement.scrollWidth }));
  expect(dimensions.document).toBeLessThanOrEqual(dimensions.viewport);
  await expect(inspector.getByRole("heading", { name: "Validate identity boundary" })).toBeVisible();
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21aa"]).analyze();
  expect(results.violations).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath("observatory-mobile.png"), fullPage: true });
});

test("keeps observatory keyboard access, reduced motion, and empty state honest", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await boot(page, true);
  await page.getByRole("link", { name: "Observatory" }).click();
  await expect(page.getByText("Reduced motion", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: /Agent Room.*0 tasks/ }).focus();
  await page.keyboard.press("Enter");
  await expect(page.getByText("No tasks have been recorded for this project.")).toBeVisible();
  await expect(page.getByText("Nothing waiting for you in this project.")).toBeVisible();
  await page.setViewportSize({ width: 320, height: 900 });
  await page.getByText("Connection details", { exact: true }).click();
  await expect(page.getByText(/Connection opened/)).toBeVisible();
  await expect(page.getByText("Last message received Waiting for a message")).toBeVisible();
  const dimensions = await page.evaluate(() => ({ viewport: document.documentElement.clientWidth, document: document.documentElement.scrollWidth }));
  expect(dimensions.document).toBeLessThanOrEqual(dimensions.viewport);
  const results = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21aa"]).analyze();
  expect(results.violations).toEqual([]);
});

for (const viewport of [
  { width: 1440, height: 1000, label: "wide desktop" },
  { width: 1024, height: 768, label: "compact desktop" },
]) {
  test(`is accessible and does not overflow at ${viewport.label}`, async ({ page }, testInfo) => {
    await page.setViewportSize({ width: viewport.width, height: viewport.height });
    await boot(page);
    await page.getByText("Connection details", { exact: true }).click();
    await expect(page.getByText(/Connection opened/)).toBeVisible();
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

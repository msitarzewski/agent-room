import { act, cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { axe } from "jest-axe";
import { vi } from "vitest";
import { App } from "../App";
import { AuthProvider, safeLoginReturnTo } from "../auth/AuthContext";
import { api } from "../api/client";
import type { Project, ProjectBrief, SemanticEvent } from "../api/types";
import { STREAM_REFRESH_DELAY } from "../stream/StreamContext";
import {
  artifact,
  approval,
  attention,
  authenticatedSession,
  budget,
  endpointItems,
  health,
  projectBrief,
  run,
  semanticEvent,
  task,
} from "./fixtures";

interface FetchOptions {
  empty?: boolean;
  deniedPath?: string;
  errorPath?: string;
  authenticated?: boolean;
  degradedHealth?: boolean;
  nullProjects?: boolean;
  brief?: ProjectBrief;
  projects?: Project[];
  events?: SemanticEvent[];
}

interface ControlledSocket {
  readyState: number;
  emitOpen: () => void;
  emitMessage: (data: unknown) => void;
  emitClose: (code?: number) => void;
}

function sockets(): ControlledSocket[] {
  return (globalThis as typeof globalThis & { __testSockets: ControlledSocket[] }).__testSockets;
}

function mockApi(options: FetchOptions = {}) {
  const calls: Array<{ url: string; params: URLSearchParams; init?: RequestInit }> = [];
  vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const rawUrl = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    const url = new URL(rawUrl, window.location.origin);
    calls.push({ url: url.pathname, params: url.searchParams, init });
    if (url.pathname === "/api/v1/auth/session") {
      if (options.authenticated === false) {
        return Promise.resolve(Response.json({ title: "Unauthorized", status: 401 }, { status: 401 }));
      }
      return Promise.resolve(Response.json(authenticatedSession));
    }
    if (url.pathname === options.deniedPath) {
      return Promise.resolve(Response.json({ title: "Forbidden", status: 403 }, { status: 403 }));
    }
    if (url.pathname === options.errorPath) {
      return Promise.resolve(Response.json({ title: "Unavailable", detail: "Adapter is offline.", status: 503 }, { status: 503 }));
    }
    if (url.pathname === "/api/v1/brief/acknowledge" && init?.method === "POST") {
      if (typeof init.body !== "string") throw new Error("Expected a JSON review command.");
      const command = JSON.parse(init.body) as { through_cursor: number };
      return Promise.resolve(Response.json({ reviewed_cursor: command.through_cursor, replayed: false }));
    }
    if (url.pathname === "/api/v1/projects" && options.nullProjects) {
      return Promise.resolve(Response.json({ items: null }));
    }
    if (url.pathname === "/api/v1/projects" && options.projects) {
      return Promise.resolve(Response.json({ items: options.projects }));
    }
    if (url.pathname === "/api/v1/events") {
      const query = url.searchParams.get("q")?.toLowerCase() ?? "";
      const cursor = url.searchParams.get("cursor");
      const eventType = url.searchParams.get("event_type");
      const limit = Number(url.searchParams.get("limit") ?? 100);
      const events = options.empty ? [] : options.events ?? [semanticEvent];
      const projectEvents = events.filter((event) => event.project_id === url.searchParams.get("project_id"));
      // The fixture timestamps use UTC. Padding the fraction preserves microseconds and equal instants.
      const timestamp = (value: string) => value.slice(0, 19) + value.slice(20, -1).padEnd(6, "0");
      const ordered = projectEvents.sort((a, b) => timestamp(b.occurred_at).localeCompare(timestamp(a.occurred_at)) || b.cursor - a.cursor);
      const start = cursor === null ? 0 : ordered.findIndex((event) => String(event.cursor) === cursor) + 1;
      const matches = ordered.slice(start).filter((event) => (!eventType || event.type === eventType) &&
        [event.type, event.subject_type, event.subject_id, event.actor_id].some((field) => field.toLowerCase().includes(query)));
      const items = matches.slice(0, limit);
      const event_types = url.searchParams.get("include_types") === "true"
        ? [...new Set(projectEvents.map((event) => event.type))].sort() : undefined;
      return Promise.resolve(Response.json({ items, next_cursor: matches.length > limit ? items.at(-1)?.cursor : null, event_types }));
    }
    if (init?.method === "POST") {
      return Promise.resolve(Response.json({ id: "updated", version: 2, project_id: "project_alpha", updated_at: new Date().toISOString() }));
    }
    if (url.pathname === "/api/v1/health/components") {
      return Promise.resolve(Response.json(options.degradedHealth
        ? { ...health, artifacts: { status: "error" } }
        : health));
    }
    if (url.pathname === "/api/v1/brief") {
      return Promise.resolve(Response.json(options.brief ?? projectBrief));
    }
    const items = options.empty ? [] : (endpointItems[url.pathname] ?? []);
    return Promise.resolve(Response.json({ items, next_cursor: null }));
  }));
  return calls;
}

async function renderPath(path: string, options: FetchOptions = {}) {
  localStorage.setItem("agent-room:project", "project_alpha");
  window.history.pushState({}, "", path);
  const calls = mockApi(options);
  const result = render(<AuthProvider><App /></AuthProvider>);
  await waitFor(() => expect(screen.queryByText("Opening Agent Room")).not.toBeInTheDocument());
  return { ...result, calls };
}

// Sparse cursors reflect writes for other projects in the same event log.
function timelineHistory(count: number): SemanticEvent[] {
  const latest = Date.parse(semanticEvent.occurred_at);
  return Array.from({ length: count }, (_, index) => ({
    ...semanticEvent, id: `event_${index + 1}`, cursor: (index + 1) * 3, subject_id: `task_${index + 1}`,
    occurred_at: new Date(latest - (count - 1 - index) * 1_000).toISOString(),
  }));
}

describe("Agent Room application", () => {
  it("offers only the production OIDC sign-in flow and is accessible", async () => {
    const { container } = await renderPath("/", { authenticated: false });
    expect(screen.getByRole("heading", { name: "The work continues. The room remembers." })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Enter quietly" })).toBeInTheDocument();
    expect(screen.getByText("If you are expected, the door already knows.")).toBeInTheDocument();
    expect(screen.queryByLabelText(/password/i)).not.toBeInTheDocument();
    expect((await axe(container)).violations).toEqual([]);
  });

  it("preserves the legacy login route without disclosing the identity provider", async () => {
    await renderPath("/login", { authenticated: false });
    expect(screen.getByRole("heading", {
      name: "The work continues. The room remembers.",
    })).toBeInTheDocument();
    expect(window.location.pathname).toBe("/login");
    expect(document.body).not.toHaveTextContent(/identity provider|network membership|authorization code/i);
  });

  it("preserves an unauthenticated protected destination while presenting the public threshold", async () => {
    await renderPath("/workspace?project=project_alpha&task=task_1", { authenticated: false });
    expect(await screen.findByRole("heading", {
      name: "The work continues. The room remembers.",
    })).toBeInTheDocument();
    expect(window.location.pathname).toBe("/workspace");
    expect(window.location.search).toBe("?project=project_alpha&task=task_1");
    const loginUrl = vi.spyOn(api, "loginUrl").mockReturnValue("#oidc-start");
    await userEvent.setup().click(screen.getByRole("button", { name: "Enter quietly" }));
    expect(loginUrl).toHaveBeenCalledWith("/workspace?project=project_alpha&task=task_1");
  });

  it("defaults root and legacy login entry to the root without a login loop", async () => {
    const loginUrl = vi.spyOn(api, "loginUrl").mockReturnValue("#oidc-start");
    const first = await renderPath("/", { authenticated: false });
    await userEvent.setup().click(screen.getByRole("button", { name: "Enter quietly" }));
    expect(loginUrl).toHaveBeenLastCalledWith("/");
    first.unmount();
    await renderPath("/login?provider=ignored", { authenticated: false });
    await userEvent.setup().click(screen.getByRole("button", { name: "Enter quietly" }));
    expect(loginUrl).toHaveBeenLastCalledWith("/");
  });

  it("preserves local query/fragment context and rejects unsafe or looping OIDC destinations", () => {
    expect(safeLoginReturnTo("/workspace?project=alpha&task=task_1#evidence")).toBe("/workspace?project=alpha&task=task_1#evidence");
    for (const target of ["", "https://outside.example/path", "//outside.example", "/\\outside.example", "/%2f%2foutside.example", "/%5coutside.example", "/login", "/safe/../login?again=yes", "/api/v1/auth/login", "/bad%escape", "/\noutside.example"]) {
      expect(safeLoginReturnTo(target), target).toBe("/");
    }
  });

  it("keeps a first login without project membership usable", async () => {
    await renderPath("/", { nullProjects: true });
    expect(await screen.findByRole("heading", { name: "What needs you now" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "No authorized projects" })).toBeInTheDocument();
    expect(screen.getByLabelText("Project")).toBeDisabled();
    expect(screen.queryByRole("heading", { name: "Agent Room is unreachable" })).not.toBeInTheDocument();
  });

  it("renders the attention inbox, capability-gated actions, and CSRF mutation", async () => {
    const user = userEvent.setup();
    const { container, calls } = await renderPath("/");
    expect(await screen.findByRole("heading", { name: "What needs you now" })).toBeInTheDocument();
    expect(screen.getByText("Review authentication evidence")).toBeInTheDocument();
    expect(screen.getByText("Project coordination update")).toBeInTheDocument();
    expect(screen.getByText(/work events · .* source checks · .* task reports/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Mark changes reviewed" }));
    await screen.findByText(/Changes through event 42 were marked reviewed/);
    const briefAck = calls.find((call) => call.url === "/api/v1/brief/acknowledge");
    expect(briefAck?.init?.body).toBe(JSON.stringify({ expected_cursor: 40, through_cursor: 42 }));
    expect(localStorage.getItem("agent-room:brief-cursor:project_alpha")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Acknowledge" }));
    await screen.findByText(/was acknowledged/);
    const mutation = calls.find((call) =>
      call.url === `/api/v1/attention/${attention.id}/acknowledge` && call.init?.method === "POST"
    );
    expect(new Headers(mutation?.init?.headers).get("X-CSRF-Token")).toBe("test-csrf");
    expect(new Headers(mutation?.init?.headers).get("If-Match")).toBe("1");
    expect((await axe(container)).violations).toEqual([]);
  });

  it("separates routine source checks and task reports from work events without changing review cursors", async () => {
    const brief = { ...projectBrief, through_cursor: 51, event_counts: { "project.observed": 7, "task.observed": 3, "review.requested": 1 } };
    const { calls } = await renderPath("/", { brief });
    expect(await screen.findByText("1 work events · 7 source checks · 3 task reports")).toBeInTheDocument();
    expect(screen.getByText("project.observed")).toBeInTheDocument();
    expect(screen.getByText("task.observed")).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "Mark changes reviewed" }));
    await screen.findByText(/Changes through event 51 were marked reviewed/);
    expect(calls.find((call) => call.url === "/api/v1/brief/acknowledge")?.init?.body).toBe(JSON.stringify({ expected_cursor: 40, through_cursor: 51 }));
  });

  it("shows real run controls only when capabilities advertise them", async () => {
    const user = userEvent.setup();
    const { calls } = await renderPath("/workers");
    expect(await screen.findByRole("heading", { name: "Workers & runs" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Pause" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Pause" }));
    await screen.findByText(/pause command accepted/);
    await user.click(screen.getByRole("button", { name: "Message" }));
    await user.type(screen.getByLabelText("Message for the next safe turn"), "Report the failing check.");
    await user.click(screen.getByRole("button", { name: "Send message" }));
    await screen.findByText(/message command accepted/);
    await user.click(screen.getByRole("button", { name: "Redirect" }));
    await user.type(screen.getByLabelText("New direction for this run"), "Focus on the root cause.");
    await user.click(screen.getByRole("button", { name: "Send redirect" }));
    await screen.findByText(/redirect command accepted/);
    await user.click(screen.getByRole("button", { name: "Cancel run" }));
    expect(screen.getByText(/Cancel Build secure session flow/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Submit approved cancellation" }));
    await screen.findByText(/cancel command accepted/);
    expect(screen.queryByText(/Cancel Build secure session flow/)).not.toBeInTheDocument();
    expect(calls.filter((call) => call.url.includes("/runs/run_1/actions")).length).toBe(4);
    expect(calls.filter((call) => call.url.includes("/runs/run_1/actions")).at(-1)?.init?.body)
      .toBe(JSON.stringify({ action: "cancel", approval_id: "approval_cancel_run_1" }));
  });

  it("requests approval instead of executing an unapproved cancellation", async () => {
    const previous = endpointItems["/api/v1/approvals"];
    endpointItems["/api/v1/approvals"] = [];
    const user = userEvent.setup();
    const { calls } = await renderPath("/workers");
    await user.click(await screen.findByRole("button", { name: "Request cancellation approval" }));
    await screen.findByText(/Cancellation approval requested/);
    expect(calls.some((call) => call.url === "/api/v1/approvals" && call.init?.method === "POST")).toBe(true);
    expect(calls.some((call) =>
      call.url.includes("/runs/run_1/actions") &&
      typeof call.init?.body === "string" &&
      call.init.body.includes("cancel"),
    )).toBe(false);
    endpointItems["/api/v1/approvals"] = previous ?? [];
  });

  it("rejects an approved cancellation bound to a stale run version", async () => {
    const previous = endpointItems["/api/v1/approvals"];
    endpointItems["/api/v1/approvals"] = [{ ...approval, expected_target_version: run.version + 1 }];
    const user = userEvent.setup();
    const { calls } = await renderPath("/workers");

    expect(await screen.findByRole("button", { name: "Request cancellation approval" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancel run" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Request cancellation approval" }));
    const request = calls.find((call) => call.url === "/api/v1/approvals" && call.init?.method === "POST");
    expect(request?.init?.body).toContain(`"expected_target_version":${run.version}`);
    endpointItems["/api/v1/approvals"] = previous ?? [];
  });

  it("reviews a completion claim with the canonical decision value", async () => {
    const user = userEvent.setup();
    const { calls } = await renderPath("/tasks");
    expect(await screen.findByRole("heading", { name: "Tasks & review" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Approve evidence" }));
    await screen.findByText(/was approved/);
    const review = calls.find((call) => call.url.includes("/claims/"));
    expect(review?.init?.body).toBe(JSON.stringify({ decision: "accepted" }));
    await user.click(screen.getByRole("button", { name: "Reject claim" }));
    expect(calls.filter((call) => call.url.includes("/claims/")).at(-1)?.init?.body).toBe(JSON.stringify({ decision: "rejected" }));
  });

  it("confirms blocked transitions with a durable reason", async () => {
    const user = userEvent.setup();
    const { calls } = await renderPath("/tasks");
    await screen.findByText("Build secure session flow");
    await user.click(screen.getByRole("button", { name: "Move to blocked" }));
    expect(screen.getByRole("alertdialog", { name: /Why is Build secure session flow blocked/ })).toBeInTheDocument();
    await user.type(screen.getByLabelText("Blocking reason"), "Waiting for operator approval.");
    await user.click(screen.getByRole("button", { name: "Confirm blocked state" }));
    await screen.findByText(/was blocked/);
    const transition = calls.find((call) => call.url.includes("/tasks/task_1/transition"));
    expect(transition?.init?.body).toBe(JSON.stringify({ status: "blocked", reason: "Waiting for operator approval." }));
  });

  it("reviews the exact approval target before deciding", async () => {
    const previous = endpointItems["/api/v1/approvals"];
    endpointItems["/api/v1/approvals"] = [{ ...approval, status: "pending" }];
    const user = userEvent.setup();
    const { calls } = await renderPath("/approvals");
    await user.click(await screen.findByRole("button", { name: "Review to approve" }));
    expect(screen.getByRole("alertdialog", { name: "Approve exact action?" })).toHaveTextContent("run:run_1");
    await user.type(screen.getByLabelText("Decision note"), "Target and digest verified.");
    await user.click(screen.getByRole("button", { name: "Confirm approval" }));
    await screen.findByText(/was approved/);
    const mutation = calls.find((call) => call.url.includes("/approvals/approval_cancel_run_1/decision"));
    expect(mutation?.init?.body).toBe(JSON.stringify({ decision: "approved", note: "Target and digest verified." }));
    endpointItems["/api/v1/approvals"] = previous ?? [];
  });

  it.each([
    ["/timeline", "Timeline", "task · task_1"],
    ["/approvals", "Approvals", "cancel · run_1"],
    ["/evidence", "Evidence & artifacts", "Authentication test report"],
    ["/chat", "Chat", "Pip"],
    ["/budgets", "Costs & budgets", "Daily token budget"],
    ["/health", "Health & connectivity", "PostgreSQL"],
    ["/sessions", "Active sessions", "Agent Room release"],
  ])("renders %s with live API data", async (path, heading, content) => {
    await renderPath(path);
    expect(await screen.findByRole("heading", { name: heading })).toBeInTheDocument();
    expect((await screen.findAllByText(content)).length).toBeGreaterThan(0);
  });

  it("surfaces degraded health without exposing private diagnostics", async () => {
    await renderPath("/health", { degradedHealth: true });
    expect(await screen.findByText("1 components need attention")).toBeInTheDocument();
    expect(screen.getByText("Artifact store")).toBeInTheDocument();
    expect(screen.getByText("Content store check failed")).toBeInTheDocument();
  });

  it("posts chat as a message without exposing a command control", async () => {
    const user = userEvent.setup();
    const { calls } = await renderPath("/chat");
    await screen.findByText("I found one review that needs your attention.");
    await user.type(screen.getByLabelText("Message this project"), "Please summarize the evidence.");
    await user.click(screen.getByRole("button", { name: "Send message" }));
    await screen.findByText("Message posted.");
    const post = calls.find((call) => call.url === "/api/v1/chat/messages" && call.init?.method === "POST");
    expect(post?.init?.body).toContain("Please summarize");
    expect(screen.queryByRole("button", { name: /execute/i })).not.toBeInTheDocument();
  });

  it("renders the empty state without production sample data", async () => {
    await renderPath("/", { empty: true });
    expect(await screen.findByText("The room is quiet")).toBeInTheDocument();
    expect(screen.queryByText("Review authentication evidence")).not.toBeInTheDocument();
  });

  it("renders denied and operational error states with a recovery action", async () => {
    const user = userEvent.setup();
    await renderPath("/health", { deniedPath: "/api/v1/health/components" });
    expect(await screen.findByText("Access denied")).toBeInTheDocument();
    cleanup();
    await renderPath("/timeline", { errorPath: "/api/v1/events" });
    expect(await screen.findByText("Adapter is offline.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Try again" }));
  });

  it("supports keyboard navigation with a skip link and project selector", async () => {
    const user = userEvent.setup();
    await renderPath("/");
    await screen.findByRole("heading", { name: "What needs you now" });
    await user.tab();
    expect(screen.getByRole("link", { name: "Skip to content" })).toHaveFocus();
    expect(screen.getByLabelText("Project")).toHaveValue("project_alpha");
    expect(within(screen.getByRole("navigation", { name: "Primary navigation" })).getByRole("link", { name: /Attention/ })).toBeInTheDocument();
  });

  it("handles stream open, event cursor refresh, parse error, and reconnect state", async () => {
    await renderPath("/");
    await screen.findByRole("heading", { name: "What needs you now" });
    const socket = sockets().at(-1);
    expect(socket).toBeDefined();
    act(() => socket?.emitOpen());
    await screen.findByText("connected");
    act(() => socket?.emitMessage({
      cursor: 42,
      type: "task.updated",
      occurred_at: "2026-07-27T12:01:00Z",
      data: {},
    }));
    await screen.findByText(/Last project activity/);
    const activity = screen.getByText(/Last project activity/);
    expect(activity.querySelector("time")).toHaveAttribute("datetime", "2026-07-27T12:01:00Z");
    expect(screen.getByText(/Connection opened/).querySelector("time")).toBeInTheDocument();
    act(() => socket?.emitMessage({
      cursor: 43,
      type: "heartbeat",
      occurred_at: "2026-07-27T12:01:01Z",
    }));
    expect(screen.getByText(/Last message received/).querySelector("time")).toBeInTheDocument();
    expect(activity.querySelector("time")).toHaveAttribute("datetime", "2026-07-27T12:01:00Z");
    act(() => socket?.emitMessage({
      cursor: 44,
      type: "resync_required",
      occurred_at: "2026-07-27T12:01:02Z",
    }));
    act(() => socket?.emitClose());
    await screen.findByText("reconnecting");
  });

  it("revokes the session and closes the project stream on logout", async () => {
    const user = userEvent.setup();
    await renderPath("/");
    await screen.findByRole("heading", { name: "What needs you now" });
    const socket = sockets().at(-1);
    await user.click(screen.getByRole("button", { name: "Sign out" }));
    await screen.findByRole("heading", { name: "The work continues. The room remembers." });
    expect(socket?.readyState).toBe(WebSocket.CLOSED);
  });

  it("renders untrusted chat payload as inert text", async () => {
    const previous = endpointItems["/api/v1/chat/messages"];
    endpointItems["/api/v1/chat/messages"] = [{
      ...(previous?.[0] as object),
      id: "message_xss",
      body: '<img src=x onerror="window.__pwned=true"><script>alert(1)</script>',
    }];
    await renderPath("/chat");
    expect(await screen.findByText(/<img src=x/)).toBeInTheDocument();
    expect(document.querySelector(".chat-message img")).toBeNull();
    expect(document.querySelector(".chat-message script")).toBeNull();
    endpointItems["/api/v1/chat/messages"] = previous ?? [];
  });

  it("renders resolved, read-only, degraded, exhausted, and untrusted edge states honestly", async () => {
    endpointItems["/api/v1/attention"] = [{
      ...attention,
      status: "resolved",
      owner_id: undefined,
    }];
    await renderPath("/");
    expect(await screen.findByText("Unassigned")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Acknowledge" })).not.toBeInTheDocument();
    cleanup();
    endpointItems["/api/v1/attention"] = [attention];

    endpointItems["/api/v1/runs"] = [{ ...run, status: "paused", capabilities: ["run:resume"] }];
    await renderPath("/workers");
    expect(await screen.findByRole("button", { name: "Resume" })).toBeInTheDocument();
    cleanup();
    endpointItems["/api/v1/runs"] = [run];

    endpointItems["/api/v1/tasks"] = [{
      ...task,
      status: "blocked",
      description: undefined,
      blocked_reason: "Waiting for a policy decision.",
    }];
    const projects = endpointItems["/api/v1/projects"];
    endpointItems["/api/v1/projects"] = [{ ...(projects?.[0] as object), capabilities: [] }];
    await renderPath("/tasks");
    expect(await screen.findByText(/Waiting for a policy decision/)).toBeInTheDocument();
    expect(screen.getByText("Read-only for your role.")).toBeInTheDocument();
    cleanup();
    endpointItems["/api/v1/tasks"] = [task];
    endpointItems["/api/v1/projects"] = projects ?? [];

    endpointItems["/api/v1/artifacts"] = [{ ...artifact, uri: "https://untrusted.example/artifact" }];
    await renderPath("/evidence");
    const download = await screen.findByRole("link", { name: "Download verified artifact" });
    expect(download).toHaveAttribute("href", "/api/v1/artifacts/artifact_1/content?project_id=project_alpha");
    expect(download).not.toHaveAttribute("href", expect.stringContaining("untrusted.example"));
    cleanup();
    endpointItems["/api/v1/artifacts"] = [artifact];

    endpointItems["/api/v1/budgets"] = [{
      ...budget,
      token_limit: 0,
      token_used: 0,
      cost_limit_cents: 10_000,
      cost_used_cents: 9_000,
      time_limit_seconds: 100,
      time_used_seconds: 100,
    }];
    await renderPath("/budgets");
    expect(await screen.findByText("0% consumed")).toBeInTheDocument();
    expect(screen.getByText("exhausted")).toBeInTheDocument();
    cleanup();
    endpointItems["/api/v1/budgets"] = [budget];

    health.artifacts.status = "degraded";
    await renderPath("/health");
    expect(await screen.findByText("1 components need attention")).toBeInTheDocument();
    expect(screen.getByText("Content store check failed")).toBeInTheDocument();
    health.artifacts.status = "ok";
  });

  it("filters timeline and worker results without losing the live source data", async () => {
    const user = userEvent.setup();
    await renderPath("/timeline");
    await screen.findByText("task · task_1");
    await user.type(screen.getByLabelText("Filter events"), "no such event");
    expect(await screen.findByText("No events match your filters.")).toBeInTheDocument();
    cleanup();
    await renderPath("/workers");
    await screen.findByText("Build secure session flow");
    await user.selectOptions(screen.getByLabelText("Run state"), "failed");
    expect(screen.getByText("No runs match this filter.")).toBeInTheDocument();
  });

  it("opens a 9,110-event timeline in one request and navigates split pages by cursor", async () => {
    const user = userEvent.setup();
    const events = timelineHistory(9110);
    // A late delivery has the largest cursor but the oldest occurrence.
    events[0] = { ...events[0]!, cursor: 40_000, type: "task.observed" };
    // Both sides of the page boundary share the same occurrence time.
    events[9059] = { ...events[9059]!, occurred_at: events[9060]!.occurred_at };
    const { calls } = await renderPath("/timeline", { events });
    const requests = () => calls.filter((call) => call.url === "/api/v1/events");
    const pager = () => within(screen.getByRole("navigation", { name: "Timeline pages" }));
    // The server asks a fresh socket to resync when its replay would exceed 1,000 events.
    const socket = sockets().at(-1)!;
    vi.useFakeTimers();
    try {
      await act(async () => {
        socket.emitOpen();
        socket.emitMessage({ type: "resync_required", data: { reason: "replay_limit" } });
        await vi.advanceTimersByTimeAsync(STREAM_REFRESH_DELAY + 1);
      });
    } finally {
      vi.useRealTimers();
    }
    await screen.findByText("task · task_9110");
    expect(requests()).toHaveLength(1);
    expect(Object.fromEntries(requests()[0]!.params)).toMatchObject({
      project_id: "project_alpha", limit: "50", order: "desc", include_types: "true",
    });
    expect(requests()[0]!.params.has("cursor")).toBe(false);
    expect(requests()[0]!.params.has("event_type")).toBe(false);
    const types = screen.getByRole("combobox", { name: "Event type" });
    expect(within(types).getByRole("option", { name: "All event types", selected: true })).toBeInTheDocument();
    expect(within(types).getByRole("option", { name: "task.observed" })).toBeInTheDocument();
    const items = within(screen.getByRole("list", { name: "Event timeline" })).getAllByRole("listitem");
    expect(items).toHaveLength(50);
    expect(items[0]).toHaveTextContent("task · task_9110");
    expect(items.at(-1)).toHaveTextContent("task · task_9061");
    expect(pager().getByRole("button", { name: "Newer" })).toBeDisabled();

    await user.click(pager().getByRole("button", { name: "Older" }));
    await screen.findByText("task · task_9060");
    expect(screen.queryByText("task · task_9061")).not.toBeInTheDocument();
    expect(requests()).toHaveLength(2);
    expect(requests()[1]!.params.get("cursor")).toBe(String(9061 * 3));
    expect(requests()[1]!.params.has("include_types")).toBe(false);

    await user.click(pager().getByRole("button", { name: "Newer" }));
    await screen.findByText("task · task_9110");
    expect(pager().getByRole("button", { name: "Newer" })).toBeDisabled();
    expect(screen.queryByText("task · task_9060")).not.toBeInTheDocument();
  });

  it("combines exact event type and text across history, and resets filters and pages on project change", async () => {
    const user = userEvent.setup();
    const events = timelineHistory(120);
    for (const event of events.slice(0, 60)) event.type = "task.observed";
    events[0] = { ...events[0]!, subject_id: "archived-approval" };
    events[1] = { ...events[1]!, type: "task.observed.extra", subject_id: "archived-approval-other-type" };
    events.push({ ...semanticEvent, project_id: "project_beta", id: "beta-event", type: "run.started", subject_id: "beta-subject", cursor: 2047 });
    const projects = ["alpha", "beta"].map((suffix) => ({
      id: `project_${suffix}`, name: suffix, version: 1, updated_at: semanticEvent.occurred_at, capabilities: ["*"],
    }));
    const { calls } = await renderPath("/timeline", { events, projects });
    await screen.findByText("task · task_120");
    await user.click(screen.getByRole("button", { name: "Older" }));
    await screen.findByText("task · task_70");
    const typeMenu = () => screen.getByRole("combobox", { name: "Event type" });
    await user.selectOptions(typeMenu(), "task.observed");
    await screen.findByText("task · task_60");
    expect(screen.getByRole("button", { name: "Newer" })).toBeDisabled();
    expect(within(typeMenu()).getByRole("option", { name: "task.observed.extra" })).toBeInTheDocument();
    await user.type(screen.getByLabelText("Filter events"), "archived-approval");
    expect(await screen.findByText("task · archived-approval")).toBeInTheDocument();
    const searchRequest = calls.filter((call) => call.url === "/api/v1/events").at(-1)!;
    expect(searchRequest.params.get("q")).toBe("archived-approval");
    expect(searchRequest.params.get("event_type")).toBe("task.observed");
    expect(searchRequest.params.has("cursor")).toBe(false);
    expect(screen.queryByText("task · archived-approval-other-type")).not.toBeInTheDocument();
    expect(screen.queryByText("task · task_60")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Older" })).toBeDisabled();

    await user.clear(screen.getByLabelText("Filter events"));
    await screen.findByText("task · task_60");
    await user.click(screen.getByRole("button", { name: "Older" }));
    await screen.findByText("task · archived-approval");
    await user.selectOptions(screen.getByLabelText("Project"), "project_beta");
    expect(await screen.findByText("task · beta-subject")).toBeInTheDocument();
    expect(screen.queryByText("task · archived-approval")).not.toBeInTheDocument();
    const betaRequests = calls.filter((call) => call.url === "/api/v1/events" && call.params.get("project_id") === "project_beta");
    expect(betaRequests).toHaveLength(1);
    expect(betaRequests[0]!.params.has("cursor")).toBe(false);
    expect(betaRequests[0]!.params.get("include_types")).toBe("true");
    expect(betaRequests[0]!.params.has("event_type")).toBe(false);
    expect(within(typeMenu()).getByRole("option", { name: "All event types", selected: true })).toBeInTheDocument();
    expect(within(typeMenu()).getByRole("option", { name: "run.started" })).toBeInTheDocument();
    expect(within(typeMenu()).queryByRole("option", { name: "task.observed" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Newer" })).toBeDisabled();
  });

  it("keeps an older timeline page stable during live activity and hides it on revocation", async () => {
    const user = userEvent.setup();
    const events = timelineHistory(51);
    const { calls } = await renderPath("/timeline", { events });
    await screen.findByText("task · task_51");
    await user.click(screen.getByRole("button", { name: "Older" }));
    await screen.findByText("task · task_1");
    const fetched = calls.filter((call) => call.url === "/api/v1/events").length;
    const socket = sockets().at(-1)!;
    vi.useFakeTimers();
    try {
      await act(async () => {
        socket.emitMessage({ type: "task.updated", cursor: 155, occurred_at: semanticEvent.occurred_at });
        await vi.advanceTimersByTimeAsync(STREAM_REFRESH_DELAY + 1);
      });
    } finally {
      vi.useRealTimers();
    }
    expect(screen.getByText("task · task_1")).toBeInTheDocument();
    expect(calls.filter((call) => call.url === "/api/v1/events")).toHaveLength(fetched);
    act(() => socket.emitClose(4403));
    expect(await screen.findByText("Access denied")).toBeInTheDocument();
    expect(screen.queryByText("task · task_1")).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: semanticEvent.type })).not.toBeInTheDocument();
  });

  it("merges only live events of the selected type by occurrence time, breaking ties by cursor", async () => {
    const user = userEvent.setup();
    const events = timelineHistory(51);
    const { calls } = await renderPath("/timeline", { events });
    await screen.findByText("task · task_51");
    const typeMenu = screen.getByRole("combobox", { name: "Event type" });
    await user.selectOptions(typeMenu, semanticEvent.type);
    await waitFor(() => expect(calls.filter((call) => call.url === "/api/v1/events").at(-1)?.params.get("event_type")).toBe(semanticEvent.type));
    await screen.findByText("task · task_51");
    const fetched = calls.filter((call) => call.url === "/api/v1/events").length;
    const newest = { ...semanticEvent, id: "live-newest", subject_id: "newest", cursor: 155, occurred_at: "2026-07-27T12:00:00.000900Z" };
    const earlier = { ...newest, id: "live-earlier", subject_id: "earlier", cursor: 157, occurred_at: "2026-07-27T12:00:00.000100Z" };
    const tied = { ...earlier, id: "live-tie", subject_id: "tie", cursor: 158 };
    const late = { ...newest, id: "late", subject_id: "late", cursor: 160, occurred_at: "2026-07-01T12:00:00Z" };
    const unrelated = { ...newest, type: `${semanticEvent.type}.extra`, id: "other", subject_id: "other", cursor: 161 };
    events.push(newest, earlier, tied, late, unrelated);
    const socket = sockets().at(-1)!;
    act(() => {
      for (const event of [newest, earlier, tied, late, unrelated, newest]) {
        socket.emitMessage({ type: event.type, cursor: event.cursor, occurred_at: event.occurred_at, data: event });
      }
    });
    await screen.findByText("task · newest");
    const items = within(screen.getByRole("list", { name: "Event timeline" })).getAllByRole("listitem");
    expect(items).toHaveLength(50);
    expect(items.slice(0, 4).map((item) => within(item).getByRole("heading").textContent))
      .toEqual(["task · newest", "task · tie", "task · earlier", "task · task_51"]);
    expect(screen.queryByText("task · late")).not.toBeInTheDocument();
    expect(screen.queryByText("task · other")).not.toBeInTheDocument();
    expect(within(typeMenu).getByRole("option", { name: unrelated.type })).toBeInTheDocument();
    expect(calls.filter((call) => call.url === "/api/v1/events")).toHaveLength(fetched);
    await user.click(screen.getByRole("button", { name: "Older" }));
    expect(await screen.findByText("task · late")).toBeInTheDocument();
    expect(screen.getByText("task · task_4")).toBeInTheDocument();
    expect(screen.queryByText("task · task_5")).not.toBeInTheDocument();
  });

  it("refreshes one bounded page for a burst of incomplete stream notifications", async () => {
    const events = timelineHistory(51);
    const { calls } = await renderPath("/timeline", { events });
    await screen.findByText("task · task_51");
    events.push({ ...semanticEvent, id: "new-event", subject_id: "new-task", cursor: 160 });
    const socket = sockets().at(-1)!;
    act(() => {
      socket.emitMessage({ type: "task.updated", cursor: 159, occurred_at: semanticEvent.occurred_at });
      socket.emitMessage({ type: "task.updated", cursor: 160, occurred_at: semanticEvent.occurred_at });
    });
    expect(await screen.findByText("task · new-task")).toBeInTheDocument();
    const requests = calls.filter((call) => call.url === "/api/v1/events");
    expect(requests).toHaveLength(2);
    expect(requests[1]!.params.get("limit")).toBe("50");
    expect(within(screen.getByRole("list", { name: "Event timeline" })).getAllByRole("listitem")).toHaveLength(50);
  });

  it("keeps a live event received while the latest page is loading", async () => {
    let finishSnapshot!: () => void;
    const snapshot = new Promise<void>((resolve) => { finishSnapshot = resolve; });
    const request = vi.spyOn(api, "timeline").mockImplementationOnce(async () => {
      await snapshot;
      return { items: [semanticEvent] };
    });
    const incoming = { ...semanticEvent, id: "incoming", subject_id: "new-task", cursor: semanticEvent.cursor + 1 };
    await renderPath("/timeline", { events: [semanticEvent, incoming] });
    await waitFor(() => expect(request).toHaveBeenCalled());
    act(() => sockets().at(-1)!.emitMessage({
      type: incoming.type, cursor: incoming.cursor, occurred_at: incoming.occurred_at, data: incoming,
    }));
    await act(async () => {
      finishSnapshot();
      await snapshot;
    });
    await screen.findByText("task · task_1");
    expect(await screen.findByText("task · new-task")).toBeInTheDocument();
  });

  it("enables Older when an initially empty timeline grows past one page", async () => {
    const user = userEvent.setup();
    const events: SemanticEvent[] = [];
    await renderPath("/timeline", { events });
    await screen.findByText("No semantic events");
    const socket = sockets().at(-1)!;
    events.push(...timelineHistory(51));
    act(() => {
      for (const event of events) {
        socket.emitMessage({ type: event.type, cursor: event.cursor, occurred_at: event.occurred_at, data: event });
      }
    });
    await screen.findByText("task · task_51");
    expect(screen.queryByText("task · task_1")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Older" }));
    expect(await screen.findByText("task · task_1")).toBeInTheDocument();
  });
});

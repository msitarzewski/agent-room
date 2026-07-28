import { cleanup, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { axe } from "jest-axe";
import { vi } from "vitest";
import { App } from "../App";
import { AuthProvider } from "../auth/AuthContext";
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
  task,
} from "./fixtures";

interface FetchOptions {
  empty?: boolean;
  deniedPath?: string;
  errorPath?: string;
  authenticated?: boolean;
  degradedHealth?: boolean;
  nullProjects?: boolean;
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
  const calls: Array<{ url: string; init?: RequestInit }> = [];
  vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const rawUrl = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
    const url = new URL(rawUrl, window.location.origin);
    calls.push({ url: url.pathname, init });
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
      return Promise.resolve(Response.json({ reviewed_cursor: 42, replayed: false }));
    }
    if (url.pathname === "/api/v1/projects" && options.nullProjects) {
      return Promise.resolve(Response.json({ items: null }));
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
      return Promise.resolve(Response.json(projectBrief));
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

describe("Agent Room application", () => {
  it("offers only the production OIDC sign-in flow and is accessible", async () => {
    const { container } = await renderPath("/login", { authenticated: false });
    expect(screen.getByRole("heading", { name: "Sign in" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Continue securely" })).toBeInTheDocument();
    expect(screen.queryByLabelText(/password/i)).not.toBeInTheDocument();
    expect((await axe(container)).violations).toEqual([]);
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
    socket?.emitOpen();
    await screen.findByText("connected");
    socket?.emitMessage({ cursor: 42, type: "task.updated", occurred_at: "2026-07-27T12:01:00Z", data: {} });
    await screen.findByText(/updated/);
    socket?.emitMessage({ cursor: 43, type: "heartbeat", occurred_at: "2026-07-27T12:01:01Z" });
    socket?.emitMessage({ cursor: 44, type: "resync_required", occurred_at: "2026-07-27T12:01:02Z" });
    socket?.emitClose();
    await screen.findByText("reconnecting");
  });

  it("revokes the session and closes the project stream on logout", async () => {
    const user = userEvent.setup();
    await renderPath("/");
    await screen.findByRole("heading", { name: "What needs you now" });
    const socket = sockets().at(-1);
    await user.click(screen.getByRole("button", { name: "Sign out" }));
    await screen.findByRole("heading", { name: "Sign in" });
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
    expect(screen.getByText("No events match your filters.")).toBeInTheDocument();
    cleanup();
    await renderPath("/workers");
    await screen.findByText("Build secure session flow");
    await user.selectOptions(screen.getByLabelText("Run state"), "failed");
    expect(screen.getByText("No runs match this filter.")).toBeInTheDocument();
  });
});

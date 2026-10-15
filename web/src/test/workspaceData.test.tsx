import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, vi } from "vitest";
import { api, ApiError } from "../api/client";
import type { Page, Project, ProjectOverview, StreamMessage, Task, WorkspaceSnapshot } from "../api/types";
import { loadAllPages, useCollection } from "../hooks/useCollection";
import { STREAM_EVENT, STREAM_REFRESH_DELAY, StreamProvider, useStream } from "../stream/StreamContext";
import { loadPortfolio, loadWorkspace } from "../workspace/data";
import { useWorkspace } from "../workspace/useWorkspace";
import { authenticatedSession, task } from "./fixtures";

const auth = vi.hoisted((): { projectId: string | null; session: unknown } => ({ projectId: "project_alpha", session: null }));
vi.mock("../auth/AuthContext", () => ({ useAuth: () => auth }));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((accept, fail) => { resolve = accept; reject = fail; });
  return { promise, resolve, reject };
}

function snapshot(projectId = "project_alpha", cursor = 42): WorkspaceSnapshot {
  return {
    project_id: projectId, cursor, generated_at: "2026-10-13T18:00:00Z",
    resources: { tasks: [{ ...task, project_id: projectId }], agents: [], runs: [], attention: [], evidence: [], artifacts: [], claims: [], approvals: [] },
    sources: [{ system: "sentry", last_seen: "2026-10-13T17:59:00Z" }],
  };
}

function publish(message: Partial<StreamMessage> = {}) {
  window.dispatchEvent(new CustomEvent(STREAM_EVENT, {
    detail: { project_id: "project_alpha", cursor: 43, type: "task.changed", occurred_at: "2026-10-13T18:01:00Z", ...message },
  }));
}

async function flush() {
  await act(async () => { await Promise.resolve(); });
}

afterEach(() => {
  vi.useRealTimers();
  auth.projectId = "project_alpha";
  auth.session = null;
});

describe("complete collection loading", () => {
  it("loads more than 50 records through opaque cursors and preserves cancellation", async () => {
    const records = Array.from({ length: 123 }, (_, index) => ({ ...task, id: `task_${index}` }));
    const fetch = vi.fn()
      .mockResolvedValueOnce(Response.json({ items: records.slice(0, 50), next_cursor: "opaque/one" }))
      .mockResolvedValueOnce(Response.json({ items: records.slice(50, 100), next_cursor: "opaque/two" }))
      .mockResolvedValueOnce(Response.json({ items: records.slice(100) }));
    vi.stubGlobal("fetch", fetch);
    const controller = new AbortController();

    await expect(loadAllPages(api.tasks, "project_alpha", controller.signal)).resolves.toEqual(records);
    expect(fetch).toHaveBeenCalledTimes(3);
    expect(fetch).toHaveBeenNthCalledWith(2, "/api/v1/tasks?project_id=project_alpha&cursor=opaque%2Fone", expect.objectContaining({ signal: controller.signal }));
    expect(fetch).toHaveBeenNthCalledWith(3, "/api/v1/tasks?project_id=project_alpha&cursor=opaque%2Ftwo", expect.anything());
  });

  it("stops a repeating pagination cursor instead of issuing an endless request", async () => {
    const loader = vi.fn().mockResolvedValue({ items: [task], next_cursor: "same" });
    await expect(loadAllPages<Task>(loader, "project_alpha")).rejects.toThrow("repeated a collection cursor");
    expect(loader).toHaveBeenCalledTimes(2);
  });

  it("does not request the next page after an aborted response", async () => {
    const first = deferred<Page<Task>>();
    const loader = vi.fn().mockReturnValue(first.promise);
    const controller = new AbortController();
    const loading = loadAllPages<Task>(loader, "project_alpha", controller.signal);
    controller.abort();
    first.resolve({ items: [task], next_cursor: "next" });
    await expect(loading).rejects.toMatchObject({ name: "AbortError" });
    expect(loader).toHaveBeenCalledTimes(1);
  });

  it("advances the numeric recorded-event cursor without requesting invented history", async () => {
    const fetch = vi.fn()
      .mockResolvedValueOnce(Response.json({ items: [{ cursor: 100 }], next_cursor: 100 }))
      .mockResolvedValueOnce(Response.json({ items: [{ cursor: 101 }] }));
    vi.stubGlobal("fetch", fetch);
    const loadEvents = (projectId: string, signal?: AbortSignal, cursor?: string | number) =>
      api.events(projectId, cursor === undefined ? undefined : Number(cursor), signal);
    await expect(loadAllPages(loadEvents, "project_alpha")).resolves.toEqual([{ cursor: 100 }, { cursor: 101 }]);
    expect(fetch).toHaveBeenNthCalledWith(2, "/api/v1/events?project_id=project_alpha&cursor=100", expect.anything());
  });

  it("drops an old project's response even if its transport ignores abort", async () => {
    const first = deferred<Page<Task>>();
    const second = deferred<Page<Task>>();
    const loader = vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const hook = renderHook(() => useCollection<Task>(loader));
    auth.projectId = "project_beta";
    hook.rerender();
    expect(hook.result.current.items).toEqual([]);
    expect(loader.mock.calls[0]?.[1]).toMatchObject({ aborted: true });
    await act(async () => { second.resolve({ items: [{ ...task, project_id: "project_beta", id: "new" }] }); await second.promise; });
    await act(async () => { first.resolve({ items: [task] }); await first.promise; });
    expect(hook.result.current.items[0]?.id).toBe("new");
    expect(hook.result.current.state).toBe("ready");
  });
});

describe("consistent workspace snapshots", () => {
  it("refreshes the latest source report without replacing human task fields", async () => {
    const initial = snapshot();
    const human = { ...task, title: "Human release title", description: "Human review: 8/10 checks recorded.", source_status: "Upcoming",
      source_observation: { external_id: "release-source", title: "Source release", status: "Upcoming", description: "Source verification: 8/10 checks." } };
    initial.resources.tasks = [human];
    const latest = { ...initial, cursor: 43, resources: { ...initial.resources, tasks: [{ ...human, source_status: "Done", source_observed_at: "2026-10-14T12:00:00Z",
      source_observation: { ...human.source_observation, title: "Source release: 10/10", status: "Done", description: "Latest source verification: 10/10 checks passed." } }] } };
    vi.spyOn(api, "workspace").mockResolvedValueOnce(initial).mockResolvedValueOnce(latest);
    const hook = renderHook(() => useWorkspace("project_alpha"));
    await waitFor(() => expect(hook.result.current.state).toBe("ready"));
    await act(async () => { await hook.result.current.refresh(); });
    expect(hook.result.current.snapshot?.resources.tasks[0]).toMatchObject({
      title: human.title, description: human.description, status: human.status, owner_id: human.owner_id,
      source_status: "Done", source_observation: { title: "Source release: 10/10", description: "Latest source verification: 10/10 checks passed." },
    });
    expect(initial.resources.tasks[0]?.source_observation?.description).toBe("Source verification: 8/10 checks.");
  });

  it("requests the cancellable workspace endpoint and verifies resource project scope", async () => {
    const fetch = vi.fn().mockResolvedValue(Response.json(snapshot()));
    vi.stubGlobal("fetch", fetch);
    const controller = new AbortController();
    await expect(loadWorkspace("project_alpha", controller.signal)).resolves.toEqual(snapshot());
    expect(fetch).toHaveBeenCalledWith("/api/v1/workspace?project_id=project_alpha", expect.objectContaining({ signal: controller.signal, credentials: "include" }));
    vi.spyOn(api, "workspace").mockResolvedValue({ ...snapshot(), resources: { ...snapshot().resources, tasks: [{ ...task, project_id: "another" }] } });
    await expect(loadWorkspace("project_alpha", controller.signal)).rejects.toThrow("incorrectly scoped resources");
  });

  it("rejects an incorrect snapshot boundary or an incomplete collection", async () => {
    const workspace = vi.spyOn(api, "workspace").mockResolvedValue(snapshot("another"));
    await expect(loadWorkspace("project_alpha", new AbortController().signal)).rejects.toThrow("selected project");
    workspace.mockResolvedValue({ ...snapshot(), cursor: -1 });
    await expect(loadWorkspace("project_alpha", new AbortController().signal)).rejects.toThrow("selected project");
    workspace.mockResolvedValue({ ...snapshot(), resources: { ...snapshot().resources, tasks: null as unknown as Task[] } });
    await expect(loadWorkspace("project_alpha", new AbortController().signal)).rejects.toThrow("incomplete");
  });

  it("shows empty authorized workspaces and no-project state without fabricated records", async () => {
    const empty = snapshot();
    empty.resources.tasks = [];
    const workspace = vi.spyOn(api, "workspace").mockResolvedValue(empty);
    const hook = renderHook<ReturnType<typeof useWorkspace>, { id: string | null }>(({ id }) => useWorkspace(id), { initialProps: { id: "project_alpha" } });
    await waitFor(() => expect(hook.result.current.state).toBe("empty"));
    expect(hook.result.current.snapshot).toEqual(empty);
    hook.rerender({ id: null });
    expect(hook.result.current.snapshot).toBeNull();
    expect(hook.result.current.state).toBe("empty");
    expect(workspace).toHaveBeenCalledTimes(1);
  });

  it("does not expose previous-project data while the next project's request is pending", async () => {
    const old = deferred<WorkspaceSnapshot>();
    const next = deferred<WorkspaceSnapshot>();
    vi.spyOn(api, "workspace").mockReturnValueOnce(old.promise).mockReturnValueOnce(next.promise);
    const hook = renderHook(({ id }) => useWorkspace(id), { initialProps: { id: "project_alpha" } });
    hook.rerender({ id: "project_beta" });
    await act(async () => { next.resolve(snapshot("project_beta", 99)); await next.promise; });
    await act(async () => { old.resolve(snapshot()); await old.promise; });
    expect(hook.result.current.snapshot?.project_id).toBe("project_beta");
    expect(hook.result.current.snapshot?.cursor).toBe(99);
  });

  it("retains the last snapshot with an explicit stale error and recovers on retry", async () => {
    const workspace = vi.spyOn(api, "workspace").mockResolvedValue(snapshot());
    const hook = renderHook(() => useWorkspace("project_alpha"));
    await waitFor(() => expect(hook.result.current.state).toBe("ready"));
    workspace.mockRejectedValueOnce(new Error("Source unavailable."));
    await act(async () => { await hook.result.current.refresh(); });
    expect(hook.result.current).toMatchObject({ state: "error", error: "Source unavailable.", stale: true, refreshing: false });
    expect(hook.result.current.snapshot).toEqual(snapshot());
    await act(async () => { await hook.result.current.refresh(); });
    expect(hook.result.current).toMatchObject({ state: "ready", error: null, stale: false });
  });

  it("clears retained protected data after an HTTP authorization denial", async () => {
    const workspace = vi.spyOn(api, "workspace").mockResolvedValue(snapshot());
    const hook = renderHook(() => useWorkspace("project_alpha"));
    await waitFor(() => expect(hook.result.current.state).toBe("ready"));
    workspace.mockRejectedValueOnce(new ApiError({ title: "Forbidden", status: 403 }));
    await act(async () => { await hook.result.current.refresh(); });
    expect(hook.result.current).toMatchObject({ snapshot: null, state: "denied", stale: false });
  });

  it("batches only scoped events and refreshes on resync and low event cursors", async () => {
    vi.useFakeTimers();
    const workspace = vi.spyOn(api, "workspace").mockResolvedValue(snapshot());
    const hook = renderHook(() => useWorkspace("project_alpha"));
    await flush();
    expect(hook.result.current.state).toBe("ready");
    act(() => {
      publish({ project_id: "project_beta" });
      publish({ type: "heartbeat" });
      publish({ project_id: undefined });
      vi.advanceTimersByTime(STREAM_REFRESH_DELAY);
    });
    expect(workspace).toHaveBeenCalledTimes(1);
    act(() => { for (let index = 0; index < 15; index++) publish({ cursor: 1 }); });
    act(() => { vi.advanceTimersByTime(STREAM_REFRESH_DELAY - 1); });
    expect(workspace).toHaveBeenCalledTimes(1);
    await act(async () => { vi.advanceTimersByTime(1); await Promise.resolve(); });
    expect(workspace).toHaveBeenCalledTimes(2);
    await act(async () => { publish({ type: "resync_required", cursor: 0 }); vi.advanceTimersByTime(STREAM_REFRESH_DELAY); await Promise.resolve(); });
    expect(workspace).toHaveBeenCalledTimes(3);
  });

  it("serializes event refreshes and loads again when events arrive during an in-flight snapshot", async () => {
    vi.useFakeTimers();
    const second = deferred<WorkspaceSnapshot>();
    const workspace = vi.spyOn(api, "workspace").mockResolvedValueOnce(snapshot()).mockReturnValueOnce(second.promise).mockResolvedValue(snapshot("project_alpha", 45));
    const hook = renderHook(() => useWorkspace("project_alpha"));
    await flush();
    await act(async () => { publish(); vi.advanceTimersByTime(STREAM_REFRESH_DELAY); await Promise.resolve(); });
    expect(hook.result.current.refreshing).toBe(true);
    expect(hook.result.current.stale).toBe(true);
    act(() => { publish(); publish(); vi.advanceTimersByTime(STREAM_REFRESH_DELAY); });
    expect(workspace).toHaveBeenCalledTimes(2);
    await act(async () => { second.resolve(snapshot("project_alpha", 44)); await second.promise; });
    expect(workspace).toHaveBeenCalledTimes(3);
    expect(hook.result.current.snapshot?.cursor).toBe(45);
  });

  it("revokes a pending snapshot on stream denial and removes subscriptions on unmount", async () => {
    vi.useFakeTimers();
    const first = deferred<WorkspaceSnapshot>();
    const workspace = vi.spyOn(api, "workspace").mockReturnValue(first.promise);
    const hook = renderHook(() => useWorkspace("project_alpha"));
    act(() => { publish({ type: "authorization_denied" }); });
    expect(hook.result.current.state).toBe("denied");
    await act(async () => { first.resolve(snapshot()); await first.promise; });
    expect(hook.result.current.snapshot).toBeNull();
    expect(workspace.mock.calls[0]?.[1]?.aborted).toBe(true);
    act(() => { publish(); });
    hook.unmount();
    act(() => { vi.advanceTimersByTime(STREAM_REFRESH_DELAY); });
    expect(workspace).toHaveBeenCalledTimes(1);
  });

  it("honors a manual retry queued while a denied request is still settling", async () => {
    const first = deferred<WorkspaceSnapshot>();
    const workspace = vi.spyOn(api, "workspace").mockReturnValueOnce(first.promise).mockResolvedValue(snapshot());
    const hook = renderHook(() => useWorkspace("project_alpha"));
    act(() => { publish({ type: "authorization_denied" }); });
    await act(async () => {
      const retry = hook.result.current.refresh();
      first.resolve(snapshot());
      await retry;
    });
    expect(workspace).toHaveBeenCalledTimes(2);
    expect(hook.result.current.state).toBe("ready");
  });
});

describe("authorized portfolio summaries", () => {
  const projects: Project[] = Array.from({ length: 7 }, (_, index) => ({ id: `project_${index}`, name: `Project ${index}`, version: 1, updated_at: "2026-10-13T18:00:00Z", capabilities: ["resource:read"] }));
  const overview = (id: string): ProjectOverview => ({ project_id: id, counts: { task: 81 }, event_cursor: 42, generated_at: "2026-10-13T18:00:00Z" });

  it("preserves zero-task collection metadata without substituting receipt or response time", async () => {
    const source = { system: "sentry", last_seen: "2026-10-14T12:00:00Z", last_received_at: "2026-10-14T12:00:00Z",
      last_collected_at: "2026-10-13T16:00:00Z", last_successful_collection_at: "2026-10-13T16:00:00Z", scan_status: "complete" as const,
      project_observation: { external_id: "empty-repo", name: "Source name", path: "Software/empty", status: "active", collected_at: "2026-10-13T16:00:00Z", scan_status: "complete" as const, task_count: 0 } };
    const summary = { ...overview("project_0"), counts: { task: 0 }, sources: [source] };
    vi.spyOn(api, "overview").mockResolvedValue(summary);
    const result = await loadPortfolio([projects[0]!], new AbortController().signal);
    expect(result.project_0?.overview).toEqual(summary);
    expect(result.project_0?.project.name).toBe("Project 0");
    expect(result.project_0?.overview?.sources?.[0]).toMatchObject({ last_collected_at: "2026-10-13T16:00:00Z", last_received_at: "2026-10-14T12:00:00Z", project_observation: { task_count: 0 } });
  });

  it("limits parallel requests and reports actual counts, preserving unknown and denied projects", async () => {
    const pending = projects.map(() => deferred<ProjectOverview>());
    const load = vi.spyOn(api, "overview").mockImplementation((id) => pending[Number(id.split("_")[1])]!.promise);
    const loading = loadPortfolio(projects, new AbortController().signal);
    expect(load).toHaveBeenCalledTimes(4);
    pending[0]!.resolve(overview("project_0"));
    pending[1]!.reject(new ApiError({ title: "Forbidden", status: 403 }));
    pending[2]!.reject(new Error("Unavailable"));
    pending[3]!.resolve(overview("project_3"));
    await Promise.resolve();
    await Promise.resolve();
    expect(load).toHaveBeenCalledTimes(7);
    pending[4]!.resolve(overview("project_4"));
    pending[5]!.resolve(overview("project_5"));
    pending[6]!.resolve(overview("wrong"));
    const result = await loading;
    expect(result.project_0?.overview?.counts.task).toBe(81);
    expect(result.project_1).toMatchObject({ state: "denied", overview: null });
    expect(result.project_2).toMatchObject({ state: "error", overview: null });
    expect(result.project_6).toMatchObject({ state: "error", overview: null });
    expect(Object.keys(result)).toHaveLength(7);
  });

  it("stops queued portfolio work when cancelled", async () => {
    const first = deferred<ProjectOverview>();
    const load = vi.spyOn(api, "overview").mockReturnValue(first.promise);
    const controller = new AbortController();
    const loading = loadPortfolio(projects, controller.signal);
    controller.abort();
    first.resolve(overview("project_0"));
    await expect(loading).rejects.toMatchObject({ name: "AbortError" });
    expect(load).toHaveBeenCalledTimes(4);
    await expect(loadPortfolio([], new AbortController().signal)).resolves.toEqual({});
  });
});

interface TestSocket {
  url: string;
  emitOpen: () => void;
  emitMessage: (data: unknown) => void;
  emitClose: (code?: number) => void;
}
function sockets() { return (globalThis as unknown as { __testSockets: TestSocket[] }).__testSockets; }

describe("project-scoped stream lifecycle", () => {
  it("distinguishes project occurrence, connection opening, and heartbeat receipt across reconnects", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-10-14T12:00:00Z"));
    auth.session = authenticatedSession;
    const hook = renderHook(() => useStream(), { wrapper: StreamProvider });
    const first = sockets()[0]!;
    act(() => { first.emitOpen(); });
    expect(hook.result.current).toMatchObject({ connectedAt: "2026-10-14T12:00:00.000Z", lastReceivedAt: null, lastMessageAt: null });
    act(() => { first.emitMessage({ type: "task.changed", cursor: 42, occurred_at: "2026-10-13T16:00:00Z" }); });
    expect(hook.result.current.lastMessageAt).toBe("2026-10-13T16:00:00Z");
    expect(hook.result.current.lastReceivedAt).toBe("2026-10-14T12:00:00.000Z");
    act(() => { vi.advanceTimersByTime(15_000); first.emitMessage({ type: "heartbeat", cursor: 99, occurred_at: "2026-10-14T12:00:15Z" }); });
    expect(hook.result.current.lastReceivedAt).toBe("2026-10-14T12:00:15.000Z");
    expect(hook.result.current.lastMessageAt).toBe("2026-10-13T16:00:00Z");
    expect(sessionStorage.getItem("agent-room:cursor:project_alpha")).toBe("42");
    act(() => { first.emitMessage({ type: "resync_required", cursor: 0, occurred_at: "2026-10-14T12:00:15Z" }); });
    expect(hook.result.current.lastMessageAt).toBe("2026-10-13T16:00:00Z");
    act(() => { hook.result.current.reconnect(); });
    expect(hook.result.current).toMatchObject({ connectedAt: null, lastReceivedAt: null, lastMessageAt: "2026-10-13T16:00:00Z" });
    const second = sockets()[1]!;
    act(() => { second.emitOpen(); });
    expect(hook.result.current.connectedAt).toBe("2026-10-14T12:00:15.000Z");
    act(() => { first.emitMessage({ type: "task.changed", cursor: 999, occurred_at: "2026-10-14T12:00:15Z" }); });
    expect(hook.result.current.lastReceivedAt).toBeNull();
    expect(hook.result.current.lastMessageAt).toBe("2026-10-13T16:00:00Z");
    act(() => { second.emitMessage({ type: "task.changed", cursor: 43, occurred_at: "2026-10-13T15:00:00Z" }); });
    expect(hook.result.current.lastMessageAt).toBe("2026-10-13T16:00:00Z");
    act(() => { second.emitClose(); });
    hook.unmount();
    act(() => { vi.advanceTimersByTime(30_000); });
    expect(sockets()).toHaveLength(2);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("clears connection and project timestamps on project change or sign-out", () => {
    auth.session = authenticatedSession;
    const hook = renderHook(() => useStream(), { wrapper: StreamProvider });
    act(() => { sockets()[0]!.emitOpen(); sockets()[0]!.emitMessage({ type: "task.changed", cursor: 42, occurred_at: "2026-10-14T11:00:00Z" }); });
    auth.projectId = "project_beta";
    hook.rerender();
    expect(hook.result.current).toMatchObject({ connectedAt: null, lastReceivedAt: null, lastMessageAt: null });
    act(() => { sockets()[1]!.emitOpen(); sockets()[1]!.emitMessage({ type: "heartbeat" }); });
    expect(hook.result.current.lastReceivedAt).not.toBeNull();
    auth.session = null;
    hook.rerender();
    expect(hook.result.current).toMatchObject({ status: "disconnected", connectedAt: null, lastReceivedAt: null, lastMessageAt: null });
  });

  it("scopes lifecycle/events, recovers a resync cursor, and ignores an obsolete project socket", () => {
    auth.session = authenticatedSession;
    sessionStorage.setItem("agent-room:cursor:project_alpha", "40");
    const messages: StreamMessage[] = [];
    const listener = (event: Event) => { messages.push((event as CustomEvent<StreamMessage>).detail); };
    window.addEventListener(STREAM_EVENT, listener);
    const hook = renderHook(() => useStream(), { wrapper: StreamProvider });
    const first = sockets()[0]!;
    expect(first.url).toContain("after=40");
    act(() => { first.emitOpen(); first.emitMessage({ type: "task.changed", cursor: 42, occurred_at: "now" }); });
    expect(hook.result.current.status).toBe("connected");
    expect(messages.every((message) => message.project_id === "project_alpha")).toBe(true);
    expect(messages[0]?.type).toBe("stream_connected");
    expect(sessionStorage.getItem("agent-room:cursor:project_alpha")).toBe("42");
    act(() => { first.emitMessage({ type: "task.changed", cursor: 41, occurred_at: "now" }); });
    expect(sessionStorage.getItem("agent-room:cursor:project_alpha")).toBe("42");
    act(() => { first.emitMessage({ type: "resync_required", cursor: 0, occurred_at: "now" }); });
    expect(sessionStorage.getItem("agent-room:cursor:project_alpha")).toBeNull();
    const messageCount = messages.length;
    auth.projectId = "project_beta";
    hook.rerender();
    act(() => { first.emitOpen(); first.emitMessage({ type: "task.changed", cursor: 99, occurred_at: "now" }); first.emitClose(4403); });
    expect(messages).toHaveLength(messageCount);
    expect(hook.result.current.status).toBe("connecting");
    window.removeEventListener(STREAM_EVENT, listener);
  });

  it("refreshes after reconnect and emits a terminal authorization denial", () => {
    vi.useFakeTimers();
    auth.session = authenticatedSession;
    const listener = vi.fn();
    window.addEventListener(STREAM_EVENT, listener);
    const hook = renderHook(() => useStream(), { wrapper: StreamProvider });
    act(() => { sockets()[0]!.emitOpen(); sockets()[0]!.emitClose(); });
    expect(hook.result.current.status).toBe("reconnecting");
    act(() => { vi.advanceTimersByTime(1000); sockets()[1]!.emitOpen(); });
    expect(hook.result.current.status).toBe("connected");
    expect(listener).toHaveBeenCalledTimes(2);
    act(() => { sockets()[1]!.emitClose(4403); vi.advanceTimersByTime(30_000); });
    expect(hook.result.current.status).toBe("error");
    expect(sockets()).toHaveLength(2);
    const lastEvent = listener.mock.calls.at(-1)?.[0] as CustomEvent<StreamMessage>;
    expect(lastEvent.detail).toMatchObject({ type: "authorization_denied", project_id: "project_alpha" });
    window.removeEventListener(STREAM_EVENT, listener);
  });
});

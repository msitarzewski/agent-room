import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { axe } from "jest-axe";
import { beforeEach, vi } from "vitest";
import { api } from "../api/client";
import type { Project, WorkspaceSnapshot } from "../api/types";
import { WorkspacePage } from "../pages/WorkspacePage";
import type { SceneCanvasProps } from "../workspace/SceneCanvas";
import { loadPortfolio } from "../workspace/data";
import type { PortfolioEntry } from "../workspace/data";
import { useWorkspace } from "../workspace/useWorkspace";
import { agent, artifact, attention, claim, evidence, run, task } from "./fixtures";

interface Harness {
  auth: { projects: Project[]; projectId: string | null; setProjectId: ReturnType<typeof vi.fn> };
  workspace: ReturnType<typeof useWorkspace>;
  stream: { status: string };
  sceneFails: boolean;
  sceneProps: SceneCanvasProps | null;
}
const harness = vi.hoisted((): Harness => ({
  auth: { projects: [], projectId: null, setProjectId: vi.fn() },
  workspace: { snapshot: null, state: "empty", error: null, refresh: vi.fn().mockResolvedValue(undefined), refreshing: false, stale: false },
  stream: { status: "connected" }, sceneFails: false, sceneProps: null,
}));
vi.mock("../auth/AuthContext", () => ({ useAuth: () => harness.auth }));
vi.mock("../stream/StreamContext", () => ({ useStream: () => harness.stream }));
vi.mock("../workspace/useWorkspace", () => ({ useWorkspace: vi.fn(() => harness.workspace) }));
vi.mock("../workspace/data", () => ({ loadPortfolio: vi.fn() }));
vi.mock("../workspace/SceneCanvas", () => ({
  SceneCanvas: (props: SceneCanvasProps) => {
    if (harness.sceneFails) throw new Error("GPU initialization failed.");
    harness.sceneProps = props;
    return <div data-testid="scene-boundary" data-reduced-motion={String(props.reducedMotion)}>
      <button onClick={() => props.onProjectSelect("project_alpha")}>Spatial project</button>
      <button onClick={() => props.onProjectSelect("not-authorized")}>Unknown spatial project</button>
      <button onClick={() => props.onTaskSelect("task_1")}>Spatial task</button>
      <button onClick={() => props.onBack?.()}>Spatial portfolio</button>
    </div>;
  },
}));

const projects: Project[] = [
  { id: "project_alpha", name: "Agent Room", version: 1, updated_at: "2026-10-13T18:00:00Z", capabilities: ["*"] },
  { id: "project_beta", name: "Sentry", version: 1, updated_at: "2026-10-13T18:00:00Z", capabilities: ["resource:read"] },
];

function populatedSnapshot(): WorkspaceSnapshot {
  return {
    project_id: "project_alpha", cursor: 42, generated_at: "2026-10-13T18:00:00Z",
    resources: {
      tasks: [
        { ...task, dependencies: ["dependency", "outside"], source_status: "completed", blocked_reason: "Approval required." },
        { ...task, id: "dependency", title: "Establish trust boundary", status: "working", owner_id: undefined, description: undefined, dependencies: [], review_state: undefined, approval_state: undefined },
      ],
      agents: [agent], runs: [run], attention: [attention, { ...attention, id: "resolved", status: "resolved" }],
      evidence: [evidence, { ...evidence, id: "run_evidence", task_id: undefined, summary: "Run verification", run_id: run.id, digest: undefined }, { ...evidence, id: "unrelated", task_id: "another", run_id: undefined, summary: "Unrelated evidence" }],
      artifacts: [{ ...artifact, scope_type: "task", task_id: task.id }, { ...artifact, id: "run_artifact", name: "run-output.txt", task_id: undefined, run_id: run.id }],
      claims: [claim], approvals: [],
    },
    sources: [{ system: "sentry", last_seen: "2026-10-13T17:59:00Z" }],
  };
}

function portfolioEntry(project: Project): PortfolioEntry {
  return { project, state: "ready", overview: { project_id: project.id, counts: { task: 123, attention: 4 }, event_cursor: 42, generated_at: "2026-10-13T18:00:00Z" }, error: null };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((accept, fail) => { resolve = accept; reject = fail; });
  return { promise, resolve, reject };
}

async function openPage(path = "/workspace") {
  window.history.replaceState({}, "", path);
  const result = render(<WorkspacePage />);
  await screen.findByTestId("scene-boundary");
  return result;
}

const inspector = () => within(screen.getByRole("complementary", { name: "Workspace inspector" }));
const directory = () => within(screen.getByRole("region", { name: /Your project directory|Work in this project/ }));

beforeEach(() => {
  harness.auth.projects = projects;
  harness.auth.projectId = "project_alpha";
  harness.auth.setProjectId.mockReset().mockImplementation((id: string) => { harness.auth.projectId = id; });
  harness.workspace = { snapshot: populatedSnapshot(), state: "ready", error: null, refresh: vi.fn().mockResolvedValue(undefined), refreshing: false, stale: false };
  harness.stream.status = "connected";
  harness.sceneFails = false;
  harness.sceneProps = null;
  vi.mocked(useWorkspace).mockClear();
  vi.mocked(loadPortfolio).mockReset().mockResolvedValue(Object.fromEntries(projects.map((project) => [project.id, portfolioEntry(project)])));
  window.history.replaceState({}, "", "/workspace");
});

describe("observatory navigation and evidence", () => {
  it("shows successful source collection for a zero-task project separately from recent delivery", async () => {
    const collectedAt = "2026-10-13T16:00:00Z";
    const receivedAt = "2026-10-14T12:00:00Z";
    const source = { system: "sentry", last_seen: receivedAt, last_received_at: receivedAt, last_collected_at: collectedAt,
      last_successful_collection_at: collectedAt, scan_status: "complete" as const,
      project_observation: { external_id: "repo-empty", name: "Source-reported project name", path: "Software/empty", status: "active", collected_at: collectedAt, scan_status: "complete" as const, task_count: 0 } };
    const empty = populatedSnapshot();
    empty.resources.tasks = [];
    empty.resources.attention = [];
    empty.sources = [source];
    harness.auth.projects = [projects[0]!];
    harness.workspace.snapshot = empty;
    vi.mocked(loadPortfolio).mockResolvedValue({ project_alpha: { ...portfolioEntry(projects[0]!), overview: {
      project_id: "project_alpha", counts: { task: 0 }, event_cursor: 43, generated_at: receivedAt, sources: [source],
    } } });
    await openPage();
    const card = directory().getByRole("button", { name: /Agent Room/ });
    expect(await within(card).findByText("0 tasks · 0 attention records")).toBeInTheDocument();
    expect(within(card).getByText("Complete collection · 0 tasks observed")).toBeInTheDocument();
    expect(within(card).getByText(/Source collected/).querySelector("time")).toHaveAttribute("datetime", collectedAt);
    expect(within(card).getByText(/Agent Room received/).querySelector("time")).toHaveAttribute("datetime", receivedAt);
    await userEvent.setup().click(card);
    expect(inspector().getByRole("heading", { name: "Agent Room" })).toBeInTheDocument();
    expect(inspector().getByText("Source project: Source-reported project name · active")).toBeInTheDocument();
    expect(inspector().getByText(/Source collected/).querySelector("time")).toHaveAttribute("datetime", collectedAt);
    expect(inspector().getByText(/Agent Room received/).querySelector("time")).toHaveAttribute("datetime", receivedAt);
    expect(screen.getByText("No tasks have been recorded for this project.")).toBeInTheDocument();
  });

  it("shows the latest source report separately while preserving human task metadata", async () => {
    const humanDescription = "Human release review: 8/10 checks recorded.";
    const latestDescription = "Latest source verification: 10/10 checks passed.\nDevice acceptance remains a separate review.";
    const observedAt = "2026-10-14T12:00:00Z";
    const original = { ...task, title: "Human release title", description: humanDescription, source_status: "Done", source_observed_at: observedAt,
      source_observation: { external_id: "release-source", title: "Source release: 10/10", status: "Done", description: latestDescription, external_url: "file:///workspace/release.md" } };
    harness.workspace.snapshot!.resources.tasks = [original];
    const { rerender } = await openPage("/workspace?project=project_alpha&task=task_1");
    expect(inspector().getByRole("heading", { name: original.title })).toBeInTheDocument();
    expect(inspector().getByText(humanDescription)).toBeInTheDocument();
    expect(inspector().getByText("Builder")).toBeInTheDocument();
    expect(inspector().getByText("Source reports: Done. Supervised state: review.")).toBeInTheDocument();
    const report = within(screen.getByRole("region", { name: "Latest source report" }));
    expect(report.getByText("Source release: 10/10")).toBeInTheDocument();
    expect(report.getByText(/Latest source verification: 10\/10 checks passed/)).toHaveTextContent("Device acceptance remains a separate review.");
    expect(report.getByText("Reported status: Done · codex")).toBeInTheDocument();
    expect(report.getByText("file:///workspace/release.md")).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Latest source report" }).querySelector("time")).toHaveAttribute("datetime", observedAt);
    expect(harness.sceneProps?.tasks[0]).toMatchObject({ title: original.title, description: humanDescription, status: "review", owner_id: task.owner_id, source_observation: { description: latestDescription } });

    harness.workspace.snapshot!.resources.tasks = [{ ...original, source_observed_at: undefined,
      source_observation: { external_id: "source-native-id", title: "Source title without optional details", status: "Done" } }];
    rerender(<WorkspacePage />);
    expect(report.getByText("source-native-id")).toBeInTheDocument();
    expect(report.queryByText(/Latest source verification/)).not.toBeInTheDocument();
    expect(inspector().getByText(humanDescription)).toBeInTheDocument();
  });

  it("opens a project, follows actual task/dependency/evidence links, and returns to portfolio", async () => {
    const user = userEvent.setup();
    const { container } = await openPage();
    expect(screen.getByRole("heading", { name: "Your observatory" })).toBeInTheDocument();
    expect(await screen.findAllByText("123 tasks · 4 attention records")).toHaveLength(2);
    await user.click(directory().getByRole("button", { name: /Agent Room/ }));
    expect(harness.auth.setProjectId).toHaveBeenCalledWith("project_alpha");
    expect(new URLSearchParams(window.location.search).get("project")).toBe("project_alpha");
    expect(harness.sceneProps?.activeProjectId).toBe("project_alpha");
    await user.click(directory().getByRole("button", { name: /Build secure session flow/ }));
    expect(inspector().getByRole("heading", { name: task.title })).toBeInTheDocument();
    expect(inspector().getByText("Source reports: completed. Supervised state: review.")).toBeInTheDocument();
    expect(inspector().getByText("Builder")).toBeInTheDocument();
    expect(inspector().getByText("Approval required.")).toBeInTheDocument();
    expect(inspector().getByText("Authentication test report")).toBeInTheDocument();
    expect(inspector().getByText("Run verification")).toBeInTheDocument();
    expect(inspector().queryByText("Unrelated evidence")).not.toBeInTheDocument();
    expect(inspector().getByRole("link", { name: /test-report.json/ })).toHaveAttribute("href", "/api/v1/artifacts/artifact_1/content?project_id=project_alpha");
    expect(inspector().getByRole("link", { name: /run-output.txt/ })).toBeInTheDocument();
    expect(inspector().getByText("outside · details unavailable")).toBeInTheDocument();
    expect(harness.sceneProps?.selectedTaskId).toBe(task.id);
    expect((await axe(container)).violations).toEqual([]);
    await user.click(inspector().getByRole("button", { name: "Establish trust boundary" }));
    expect(inspector().getByRole("heading", { name: "Establish trust boundary" })).toBeInTheDocument();
    expect(inspector().getByText("Unassigned")).toBeInTheDocument();
    expect(inspector().getByText("No evidence linked to this task.")).toBeInTheDocument();
    await user.click(screen.getByRole("navigation", { name: "Workspace location" }).querySelector("button")!);
    expect(screen.getByRole("heading", { name: "Your project directory" })).toBeInTheDocument();
    expect(window.location.search).toBe("");
  });

  it("searches project and task labels and clears filters when changing scale", async () => {
    const user = userEvent.setup();
    await openPage();
    await user.type(screen.getByRole("searchbox", { name: "Search projects" }), "Sentry");
    expect(directory().queryByRole("button", { name: /Agent Room/ })).not.toBeInTheDocument();
    expect(directory().getByRole("button", { name: /Sentry/ })).toBeInTheDocument();
    await user.clear(screen.getByRole("searchbox", { name: "Search projects" }));
    await user.click(screen.getByRole("button", { name: "Spatial project" }));
    await user.type(screen.getByRole("searchbox", { name: "Search tasks" }), "review");
    expect(directory().getAllByRole("listitem")).toHaveLength(1);
    await user.clear(screen.getByRole("searchbox", { name: "Search tasks" }));
    await user.type(screen.getByRole("searchbox", { name: "Search tasks" }), "missing");
    expect(screen.getByText("Nothing matches your search.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Spatial portfolio" }));
    expect(screen.getByRole("searchbox", { name: "Search projects" })).toHaveValue("");
    await user.click(screen.getByRole("button", { name: "Unknown spatial project" }));
    expect(harness.auth.setProjectId).toHaveBeenCalledTimes(1);
  });

  it("restores an authorized deep link, changes the active subscription, and ignores unauthorized links", async () => {
    harness.auth.projectId = "project_beta";
    const first = await openPage("/workspace?project=project_alpha&task=task_1");
    expect(harness.auth.setProjectId).toHaveBeenCalledWith("project_alpha");
    expect(inspector().getByRole("heading", { name: task.title })).toBeInTheDocument();
    expect(useWorkspace).toHaveBeenCalledWith("project_alpha");
    await userEvent.setup().click(within(screen.getByRole("navigation", { name: "Workspace location" })).getByRole("button", { name: "Agent Room" }));
    expect(new URLSearchParams(window.location.search).has("task")).toBe(false);
    first.unmount();
    harness.auth.setProjectId.mockClear();
    await openPage("/workspace?project=private&task=secret");
    expect(harness.auth.setProjectId).not.toHaveBeenCalled();
    expect(inspector().getByRole("heading", { name: "A little perspective." })).toBeInTheDocument();
    expect(harness.sceneProps?.activeProjectId).toBeNull();
  });

  it("follows an authenticated project-selector change without leaking the previous workspace", async () => {
    const { rerender } = await openPage("/workspace?project=project_alpha&task=task_1");
    harness.auth.projectId = "project_beta";
    rerender(<WorkspacePage />);
    expect(harness.sceneProps?.activeProjectId).toBe("project_beta");
    expect(useWorkspace).toHaveBeenLastCalledWith("project_beta");
    expect(inspector().getByRole("heading", { name: "Sentry" })).toBeInTheDocument();
    expect(inspector().queryByText("Authentication test report")).not.toBeInTheDocument();
    expect(new URLSearchParams(window.location.search).get("project")).toBe("project_beta");
    expect(new URLSearchParams(window.location.search).has("task")).toBe(false);
  });
});

describe("supervisory actions", () => {
  it("pauses only a capability-authorized active run and refreshes authoritative state", async () => {
    const pause = vi.spyOn(api, "runAction").mockResolvedValue({ ...run, status: "paused" });
    const { rerender } = await openPage("/workspace?project=project_alpha&task=task_1");
    await userEvent.setup().click(screen.getByRole("button", { name: "Pause run" }));
    await screen.findByText("Pause requested.");
    expect(pause).toHaveBeenCalledWith(run, "pause");
    expect(harness.workspace.refresh).toHaveBeenCalledTimes(1);
    harness.workspace.snapshot!.resources.runs = [{ ...run, capabilities: [] }];
    rerender(<WorkspacePage />);
    expect(screen.queryByRole("button", { name: "Pause run" })).not.toBeInTheDocument();
  });

  it("requires confirmation for accepting and rejecting completion claims, and permits cancellation", async () => {
    const review = vi.spyOn(api, "reviewClaim").mockResolvedValue({ ...claim, status: "accepted" });
    const user = userEvent.setup();
    await openPage("/workspace?project=project_alpha&task=task_1");
    await user.click(screen.getByRole("button", { name: "Review completion" }));
    expect(screen.getByRole("alertdialog", { name: "Accept completion claim?" })).toBeInTheDocument();
    expect(review).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Keep reviewing" }));
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Review completion" }));
    await user.click(screen.getByRole("button", { name: "Confirm decision" }));
    await screen.findByText("Review decision recorded.");
    expect(review).toHaveBeenNthCalledWith(1, claim, "accepted");
    await user.click(screen.getByRole("button", { name: "Reject claim" }));
    expect(screen.getByRole("alertdialog", { name: "Reject completion claim?" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Confirm decision" }));
    expect(review).toHaveBeenNthCalledWith(2, claim, "rejected");
  });

  it("reports real command failures and does not claim a successful refresh", async () => {
    const review = vi.spyOn(api, "reviewClaim").mockRejectedValueOnce(new Error("Claim version changed.")).mockRejectedValueOnce("unavailable");
    const user = userEvent.setup();
    await openPage("/workspace?project=project_alpha&task=task_1");
    await user.click(screen.getByRole("button", { name: "Reject claim" }));
    await user.click(screen.getByRole("button", { name: "Confirm decision" }));
    await screen.findByText("Claim version changed.");
    expect(harness.workspace.refresh).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Reject claim" }));
    await user.click(screen.getByRole("button", { name: "Confirm decision" }));
    await screen.findByText("The action could not be completed.");
    expect(review).toHaveBeenCalledTimes(2);
  });

  it("keeps permission, run state, and stale-data constraints on controls", async () => {
    harness.auth.projects = [{ ...projects[0]!, capabilities: ["resource:read"] }];
    const { rerender } = await openPage("/workspace?project=project_alpha&task=task_1");
    expect(screen.queryByRole("button", { name: "Pause run" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Review completion" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Acknowledge" })).not.toBeInTheDocument();
    harness.auth.projects = projects;
    harness.workspace.stale = true;
    rerender(<WorkspacePage />);
    expect(screen.getByRole("button", { name: "Pause run" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Review completion" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Acknowledge" })).toBeDisabled();
    expect(screen.getByText("Project data may be stale. Refresh before acting.")).toBeInTheDocument();
    harness.workspace.snapshot!.resources.runs = [{ ...run, status: "completed" }];
    rerender(<WorkspacePage />);
    expect(screen.queryByRole("button", { name: "Pause run" })).not.toBeInTheDocument();
  });

  it("blocks a previously opened completion decision when its supporting snapshot becomes stale", async () => {
    const review = vi.spyOn(api, "reviewClaim").mockResolvedValue(claim);
    const user = userEvent.setup();
    const { rerender } = await openPage("/workspace?project=project_alpha&task=task_1");
    await user.click(screen.getByRole("button", { name: "Review completion" }));
    harness.workspace.stale = true;
    rerender(<WorkspacePage />);
    expect(screen.getByRole("button", { name: "Confirm decision" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Confirm decision" }));
    expect(review).not.toHaveBeenCalled();
  });

  it.each(["success", "failure"])("ignores a pending action's %s feedback after navigating to another task", async (outcome) => {
    const pending = deferred<typeof run>();
    vi.spyOn(api, "runAction").mockReturnValue(pending.promise);
    const user = userEvent.setup();
    await openPage("/workspace?project=project_alpha&task=task_1");
    await user.click(screen.getByRole("button", { name: "Pause run" }));
    await user.click(inspector().getByRole("button", { name: "Establish trust boundary" }));
    await act(async () => {
      if (outcome === "success") pending.resolve(run); else pending.reject(new Error("Old task failed."));
      await pending.promise.catch(() => undefined);
    });
    expect(inspector().getByRole("heading", { name: "Establish trust boundary" })).toBeInTheDocument();
    expect(screen.queryByText("Pause requested.")).not.toBeInTheDocument();
    expect(screen.queryByText("Old task failed.")).not.toBeInTheDocument();
    expect(harness.workspace.refresh).not.toHaveBeenCalled();
  });

  it("keeps attention within reach, selects the related task, and acknowledges explicitly", async () => {
    const acknowledge = vi.spyOn(api, "attentionAction").mockResolvedValue({ ...attention, status: "acknowledged" });
    const user = userEvent.setup();
    await openPage();
    expect(screen.getAllByRole("button", { name: /Review authentication evidence/ })).toHaveLength(1);
    await user.click(screen.getByRole("button", { name: /Review authentication evidence/ }));
    expect(inspector().getByRole("heading", { name: task.title })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Acknowledge" }));
    await screen.findByText("Attention acknowledged.");
    expect(acknowledge).toHaveBeenCalledWith(attention, "acknowledge");
  });
});

describe("availability, accessibility, and motion", () => {
  it("keeps the page usable through loading, denied, failed, empty, and unknown project states", async () => {
    harness.workspace = { ...harness.workspace, snapshot: null, state: "loading" };
    const { rerender } = await openPage("/workspace?project=project_alpha");
    expect(screen.getByText("Reading project state…")).toBeInTheDocument();
    harness.workspace.state = "denied";
    rerender(<WorkspacePage />);
    expect(screen.getByRole("alert")).toHaveTextContent("Your role cannot inspect");
    harness.workspace.state = "error";
    harness.workspace.error = "Snapshot unavailable.";
    rerender(<WorkspacePage />);
    await userEvent.setup().click(screen.getByRole("button", { name: "Retry project" }));
    expect(harness.workspace.refresh).toHaveBeenCalledTimes(1);
    expect(screen.getByText("Snapshot unavailable.")).toBeInTheDocument();
    const empty = populatedSnapshot();
    empty.resources.tasks = [];
    empty.resources.attention = [];
    empty.sources = [];
    harness.workspace = { ...harness.workspace, snapshot: empty, state: "empty", error: null };
    rerender(<WorkspacePage />);
    expect(screen.getByText("No tasks have been recorded for this project.")).toBeInTheDocument();
    expect(screen.getByText("No adapter observations received.")).toBeInTheDocument();
    expect(screen.getByText("Nothing waiting for you in this project.")).toBeInTheDocument();
    harness.auth.projects = [];
    harness.auth.projectId = null;
    harness.workspace.snapshot = null;
    harness.stream.status = "reconnecting";
    rerender(<WorkspacePage />);
    expect(screen.getByText("No projects are available to your account.")).toBeInTheDocument();
    expect(screen.getByText("Live connection reconnecting")).toBeInTheDocument();
    expect(screen.getByText("0 authorized projects")).toBeInTheDocument();
  });

  it("does not fabricate portfolio counts when summaries are restricted, unavailable, or still pending", async () => {
    vi.mocked(loadPortfolio).mockResolvedValue({
      project_alpha: { project: projects[0]!, overview: null, state: "denied", error: "Forbidden" },
      project_beta: { project: projects[1]!, overview: null, state: "error", error: "Unavailable" },
    });
    await openPage();
    expect(await screen.findByText("Summary access restricted")).toBeInTheDocument();
    expect(await screen.findByText("Summary unavailable")).toBeInTheDocument();
    expect(screen.queryByText("0 tasks · 0 attention records")).not.toBeInTheDocument();
  });

  it("honors reduced motion and listens for preference changes", async () => {
    let changed: (() => void) | undefined;
    const media = { matches: true, addEventListener: vi.fn((_type: string, listener: () => void) => { changed = listener; }), removeEventListener: vi.fn() };
    vi.stubGlobal("matchMedia", vi.fn(() => media));
    const { unmount } = await openPage();
    expect(screen.getByTestId("scene-boundary")).toHaveAttribute("data-reduced-motion", "true");
    expect(screen.getByText("Reduced motion")).toBeInTheDocument();
    act(() => { media.matches = false; changed?.(); });
    expect(screen.getByTestId("scene-boundary")).toHaveAttribute("data-reduced-motion", "false");
    unmount();
    expect(media.removeEventListener).toHaveBeenCalledWith("change", changed);
  });

  it("contains a graphics failure and preserves the real task/evidence inspector", async () => {
    harness.sceneFails = true;
    const expectedConsole = vi.spyOn(console, "error").mockImplementation(() => undefined);
    window.history.replaceState({}, "", "/workspace?project=project_alpha&task=task_1");
    render(<WorkspacePage />);
    expect(await screen.findByRole("status")).toHaveTextContent("The spatial view is unavailable");
    expect(inspector().getByRole("heading", { name: task.title })).toBeInTheDocument();
    expect(inspector().getByText("Authentication test report")).toBeInTheDocument();
    expect(expectedConsole).toHaveBeenCalled();
  });

  it("shows actual zero counts after a successful empty overview", async () => {
    harness.auth.projects = [projects[0]!];
    vi.mocked(loadPortfolio).mockResolvedValue({ project_alpha: { ...portfolioEntry(projects[0]!), overview: { project_id: "project_alpha", counts: {}, event_cursor: 0, generated_at: "2026-10-13T18:00:00Z" } } });
    await openPage();
    expect(await screen.findByText("0 tasks · 0 attention records")).toBeInTheDocument();
    expect(screen.getByText("1 authorized project")).toBeInTheDocument();
  });

  it("cancels obsolete portfolio polls, surfaces errors, and recovers without accepting a stale response", async () => {
    let poll: (() => void) | undefined;
    vi.spyOn(window, "setInterval").mockImplementation((callback: TimerHandler, delay?: number) => {
      if (typeof callback === "function" && delay === 60_000) poll = callback as () => void;
      return 123;
    });
    const old = deferred<Record<string, PortfolioEntry>>();
    vi.mocked(loadPortfolio).mockReset().mockReturnValueOnce(old.promise)
      .mockRejectedValueOnce(new Error("Directory temporarily unavailable."))
      .mockResolvedValue(Object.fromEntries(projects.map((project) => [project.id, portfolioEntry(project)])));
    await openPage();
    expect(screen.getAllByText("Reading summary…")).toHaveLength(2);
    act(() => { poll?.(); });
    expect(vi.mocked(loadPortfolio).mock.calls[0]?.[1].aborted).toBe(true);
    expect(await screen.findByText("Directory temporarily unavailable.")).toBeInTheDocument();
    await act(async () => { old.resolve({ project_alpha: portfolioEntry(projects[0]!) }); await old.promise; });
    expect(screen.queryByText("123 tasks · 4 attention records")).not.toBeInTheDocument();
    act(() => { poll?.(); });
    expect(await screen.findAllByText("123 tasks · 4 attention records")).toHaveLength(2);
    expect(screen.queryByText("Directory temporarily unavailable.")).not.toBeInTheDocument();
  });
});

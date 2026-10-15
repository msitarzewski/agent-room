import { api, ApiError } from "../api/client";
import type { CoordinationItem, Project, ProjectOverview, SourceFreshness, WorkspaceSnapshot } from "../api/types";

/** A snapshot is one server-side read boundary, never a mix of collection pages. */
export async function loadWorkspace(projectId: string, signal: AbortSignal): Promise<WorkspaceSnapshot> {
  const snapshot = await api.workspace(projectId, signal);
  signal.throwIfAborted();
  if (snapshot.project_id !== projectId || !Number.isSafeInteger(snapshot.cursor) || snapshot.cursor < 0) {
    throw new Error("The workspace response does not match the selected project.");
  }
  const keys = ["tasks", "agents", "runs", "attention", "evidence", "artifacts", "claims", "approvals"] as const;
  for (const key of keys) {
    const resources = snapshot.resources?.[key];
    if (!Array.isArray(resources) || resources.some((resource) => resource.project_id !== projectId)) {
      throw new Error("The workspace response contains incomplete or incorrectly scoped resources.");
    }
  }
  if (snapshot.resources.requests !== undefined && (!Array.isArray(snapshot.resources.requests) || snapshot.resources.requests.some((item) => item.project_id !== projectId))) {
    throw new Error("The workspace response contains incorrectly scoped coordination requests.");
  }
  return snapshot;
}

export type PortfolioRegion = "coordination" | "work" | "quiet" | "unknown";
export const PORTFOLIO_PAGE_SIZE = 12;
export const SOURCE_CURRENT_WINDOW_MS = 15 * 60_000;
export const portfolioRegions: { id: PortfolioRegion; title: string; color: string; badge: string; explanation: string }[] = [
  { id: "coordination", title: "Coordination", color: "#ffc28b", badge: "◇", explanation: "Recorded blockers, requests, handoffs and reviews" },
  { id: "work", title: "Work recorded", color: "#91e3d1", badge: "△", explanation: "Unfinished work and reported runtime participation" },
  { id: "quiet", title: "Quiet", color: "#a9b8c9", badge: "○", explanation: "Complete current coverage with no open work or asks" },
  { id: "unknown", title: "Coverage unknown", color: "#b9a9db", badge: "?", explanation: "Missing, partial, stale or restricted coverage" },
];

export function sourceCoverageCurrent(sources: SourceFreshness[] | undefined, now: number): boolean {
  return Boolean(sources?.length && sources.every((source) => {
    const completeAt = Date.parse(source.last_successful_collection_at ?? "");
    const age = now - completeAt;
    return source.scan_status === "complete" && Number.isFinite(completeAt) && age >= 0 && age <= SOURCE_CURRENT_WINDOW_MS;
  }));
}

export function projectRegion(overview: ProjectOverview | null | undefined, now: number): PortfolioRegion {
  const summary = overview?.coordination;
  if (!summary) return "unknown";
  const counts = summary.counts;
  if (counts.blocked_tasks + counts.requests + counts.reviews + counts.human_escalations + counts.human_approvals + (counts.open_attention ?? 0) > 0) return "coordination";
  if (counts.running_work + counts.agent_presence + counts.unfinished_tasks + (counts.source_unfinished_tasks ?? 0) > 0) return "work";
  return summary.complete && !summary.truncated && sourceCoverageCurrent(overview?.sources, now) ? "quiet" : "unknown";
}

export interface PortfolioSignal extends CoordinationItem { project_id: string; project_name: string }
export function portfolioSignals(projects: readonly Project[], entries: Record<string, PortfolioEntry>): PortfolioSignal[] {
  const rank: Record<CoordinationItem["kind"], number> = { blocker: 0, attention: 0, request: 1, review: 2, run: 3, agent: 4, task: 5, human_approval: 0 };
  return projects.flatMap((project) => (entries[project.id]?.overview?.coordination?.items ?? []).map((item) => ({ ...item, project_id: project.id, project_name: project.name })))
    .sort((a, b) => rank[a.kind] - rank[b.kind] || a.project_id.localeCompare(b.project_id) || a.id.localeCompare(b.id));
}

export interface PortfolioEntry {
  project: Project;
  overview: ProjectOverview | null;
  state: "ready" | "error" | "denied";
  error: string | null;
}

/** Authorized projects only. Missing counts stay unknown until their request succeeds. */
export async function loadPortfolio(
  projects: readonly Project[],
  signal: AbortSignal,
): Promise<Record<string, PortfolioEntry>> {
  let next = 0;
  const entries: [string, PortfolioEntry][] = [];
  const worker = async () => {
    while (next < projects.length) {
      signal.throwIfAborted();
      const project = projects[next++];
      if (!project) return;
      let entry: PortfolioEntry;
      try {
        const overview = await api.overview(project.id, signal);
        signal.throwIfAborted();
        if (overview.project_id !== project.id) throw new Error("The overview response has an incorrect project scope.");
        entry = { project, overview, state: "ready", error: null };
      } catch (caught) {
        signal.throwIfAborted();
        entry = {
          project, overview: null,
          state: caught instanceof ApiError && [401, 403].includes(caught.status) ? "denied" : "error",
          error: caught instanceof Error ? caught.message : "Unable to load project summary.",
        };
      }
      entries.push([project.id, entry]);
    }
  };
  await Promise.all(Array.from({ length: Math.min(4, projects.length) }, () => worker()));
  signal.throwIfAborted();
  return Object.fromEntries(entries);
}

import { api, ApiError } from "../api/client";
import type { Project, ProjectOverview, WorkspaceSnapshot } from "../api/types";

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
  return snapshot;
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

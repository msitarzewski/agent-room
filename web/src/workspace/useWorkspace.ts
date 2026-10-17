import type { WorkspaceSnapshot } from "../api/types";
import { useProjectResource } from "../hooks/useCollection";
import { loadWorkspace } from "./data";

const emptyWorkspace = (snapshot: WorkspaceSnapshot) =>
  Object.values(snapshot.resources).every((resources) => resources.length === 0);

export function useWorkspace(projectId: string | null) {
  const resource = useProjectResource(projectId, loadWorkspace, emptyWorkspace);
  return {
    snapshot: resource.value,
    state: resource.state,
    error: resource.error,
    refresh: resource.refresh,
    refreshing: resource.refreshing,
    stale: resource.stale,
  };
}

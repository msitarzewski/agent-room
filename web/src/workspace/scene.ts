import type { AttentionItem, Project, Run, Task, TaskStatus } from "../api/types";

export type Point3 = [number, number, number];

export const stateColors: Record<TaskStatus, string> = {
  inbox: "#94a5bd", ready: "#9ebfff", working: "#77e5d2", review: "#c7a4ff",
  completed: "#86cfa5", archived: "#66778c", blocked: "#ffb46e", cancelled: "#758498",
  failed: "#ff7e88", reopened: "#e5c782",
};

export interface SceneNode {
  id: string;
  kind: "project" | "task";
  title: string;
  subtitle: string;
  position: Point3;
  color: string;
  active: boolean;
  attention: number;
}

export interface SceneEdge {
  id: string;
  from: string;
  to: string;
  blocked: boolean;
}

export interface WorkspaceScene {
  nodes: SceneNode[];
  edges: SceneEdge[];
  cycleAffectedTaskIds: string[];
  unresolvedDependencies: number;
  radius: number;
}

/** Identity determines geography. Reordering or adding projects never moves an existing project. */
export function identityHash(value: string): number {
  let hash = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  hash ^= hash >>> 16;
  hash = Math.imul(hash, 0x7feb352d);
  hash ^= hash >>> 15;
  return hash >>> 0;
}

export function projectPosition(id: string): Point3 {
  const hash = identityHash(id);
  const angle = (hash % 8192) * 2.399963229728653;
  const radius = 1.2 + Math.sqrt((hash >>> 13) / 524287) * 7.5;
  return [Math.cos(angle) * radius, Math.sin(angle) * radius * 0.64, -((hash >>> 24) / 255) * 1.2];
}

function isWorking(run: Run): boolean {
  return run.status === "working" || run.status === "running";
}

function isBlocked(task: Task): boolean {
  const source = task.source_status?.trim().toLowerCase();
  return task.status === "blocked" || task.status === "failed" || source === "blocked" || source === "failed";
}

function taskLayers(tasks: Task[]): { layers: Task[][]; cycleAffectedTaskIds: string[] } {
  const byId = new Map(tasks.map((task) => [task.id, task]));
  const degree = new Map<string, number>();
  const dependents = new Map<string, string[]>();
  const depths = new Map<string, number>();
  for (const task of tasks) {
    const dependencies = [...new Set(task.dependencies)].filter((id) => byId.has(id));
    degree.set(task.id, dependencies.length);
    depths.set(task.id, 0);
    for (const id of dependencies) {
      const next = dependents.get(id) ?? [];
      next.push(task.id);
      dependents.set(id, next);
    }
  }
  const queue = tasks.filter((task) => degree.get(task.id) === 0).map((task) => task.id).sort();
  for (let index = 0; index < queue.length; index += 1) {
    const id = queue[index];
    if (id === undefined) continue;
    for (const next of dependents.get(id) ?? []) {
      depths.set(next, Math.max(depths.get(next) ?? 0, (depths.get(id) ?? 0) + 1));
      const remaining = (degree.get(next) ?? 0) - 1;
      degree.set(next, remaining);
      if (remaining === 0) queue.push(next);
    }
  }
  // Kahn's remainder contains cycles AND tasks depending on those cycles. Label it honestly.
  const cycleAffectedTaskIds = tasks.filter((task) => (degree.get(task.id) ?? 0) > 0).map((task) => task.id).sort();
  const cycleLayer = Math.max(0, ...depths.values()) + 1;
  for (const id of cycleAffectedTaskIds) depths.set(id, cycleLayer);
  const layers: Task[][] = [];
  for (const task of [...tasks].sort((a, b) => a.id.localeCompare(b.id))) {
    const depth = depths.get(task.id) ?? 0;
    (layers[depth] ??= []).push(task);
  }
  return { layers, cycleAffectedTaskIds };
}

/** Only canonical entities and declared dependencies enter the visual projection. */
export function buildWorkspaceScene(
  projects: Project[], activeProjectId: string | null, tasks: Task[], runs: Run[], attention: AttentionItem[],
): WorkspaceScene {
  const openAttention = attention.filter((item) => item.status === "open");
  const project = projects.find((candidate) => candidate.id === activeProjectId);
  if (!project) {
    return {
      nodes: [...projects].sort((a, b) => a.id.localeCompare(b.id)).map((item) => {
        const count = openAttention.filter((entry) => entry.project_id === item.id).length;
        const active = runs.some((run) => run.project_id === item.id && isWorking(run));
        return {
          id: item.id, kind: "project", title: item.name, subtitle: count ? `${count} open attention` : "Open project",
          position: projectPosition(item.id), color: count ? "#ffb46e" : "#a0b9ff", attention: count, active,
        };
      }),
      edges: [], cycleAffectedTaskIds: [], unresolvedDependencies: 0, radius: 10,
    };
  }
  const projectTasks = tasks.filter((task) => task.project_id === project.id);
  const byId = new Map(projectTasks.map((task) => [task.id, task]));
  const { layers, cycleAffectedTaskIds } = taskLayers(projectTasks);
  const layerCount = Math.max(1, layers.length);
  const maxRows = 5;
  const columnSpacing = 2.7;
  const rowSpacing = 1.35;
  const columns = Array.from({ length: layerCount }, (_, depth) => Math.max(1, Math.ceil((layers[depth]?.length ?? 0) / maxRows)));
  const totalColumns = columns.reduce((sum, count) => sum + count, 0);
  const firstColumnX = -(totalColumns - 1) * columnSpacing / 2;
  const columnOffsets: number[] = [];
  let precedingColumns = 0;
  for (const count of columns) { columnOffsets.push(precedingColumns); precedingColumns += count; }
  const nodes: SceneNode[] = [{
    id: project.id, kind: "project", title: project.name, subtitle: `${projectTasks.length} tasks`,
    position: [firstColumnX - 3.05, 0, -0.5], color: "#a0b9ff", active: false,
    attention: openAttention.filter((item) => item.project_id === project.id).length,
  }];
  layers.forEach((layer, depth) => {
    layer.forEach((task, index) => {
      // Wide layers wrap in rows, bounding text density without discarding any tasks.
      const column = Math.floor(index / maxRows);
      const rows = Math.min(maxRows, layer.length - column * maxRows);
      const row = index % maxRows;
      const count = openAttention.filter((item) => item.project_id === project.id && item.resource_type === "task" && item.resource_id === task.id).length;
      nodes.push({
        id: task.id, kind: "task", title: task.title,
        subtitle: `${task.status.replaceAll("_", " ")}${task.source_status ? ` · source ${task.source_status}` : ""}`,
        position: [firstColumnX + ((columnOffsets[depth] ?? 0) + column) * columnSpacing, (row - (rows - 1) / 2) * rowSpacing, -column * 0.12],
        color: isBlocked(task) ? "#ffb46e" : stateColors[task.status], attention: count,
        active: runs.some((run) => run.project_id === project.id && run.task_id === task.id && isWorking(run)),
      });
    });
  });
  const edges: SceneEdge[] = [];
  let unresolvedDependencies = 0;
  for (const task of projectTasks) {
    for (const dependency of new Set(task.dependencies)) {
      const source = byId.get(dependency);
      if (!source) { unresolvedDependencies += 1; continue; }
      edges.push({ id: `${dependency}:${task.id}`, from: dependency, to: task.id, blocked: isBlocked(source) });
    }
  }
  const radius = Math.max(5, ...nodes.map((node) => Math.hypot(...node.position) + 1.5));
  return { nodes, edges, cycleAffectedTaskIds, unresolvedDependencies, radius };
}

export function shortLabel(value: string, max = 35): string {
  return value.length > max ? `${value.slice(0, max - 1).trimEnd()}…` : value;
}

/** Bound text density at portfolio scale while preserving selection and the strongest signals. */
export function visibleLabels(nodes: SceneNode[], selected: string | null, hovered: string | null, limit = 8): SceneNode[] {
  if (nodes.length <= 16) return nodes;
  const score = (node: SceneNode) => (node.id === selected ? 1_000_000 : 0) + (node.id === hovered ? 100_000 : 0) + Math.min(node.attention, 9999);
  return nodes.filter((node) => node.id === selected || node.id === hovered || node.attention > 0)
    .sort((a, b) => score(b) - score(a) || a.id.localeCompare(b.id)).slice(0, limit);
}

/** Canvas labels use wrapped text, never raw HTML from source records. */
export function wrapLabel(value: string, columns = 34, maxLines = 3): string[] {
  const words = value.trim().split(/\s+/);
  const lines: string[] = [];
  let line = "";
  for (const word of words) {
    if (line && line.length + word.length + 1 > columns) { lines.push(line); line = ""; }
    line = line ? `${line} ${word}` : word;
  }
  if (line) lines.push(line);
  const visible = lines.slice(0, maxLines).map((entry) => shortLabel(entry, columns));
  if (lines.length > maxLines) visible[maxLines - 1] = shortLabel(`${visible[maxLines - 1] ?? ""}…`, columns);
  return visible.length ? visible : ["Untitled"];
}

/** Read an entire evidence summary in XR; long words and URLs continue onto subsequent pages. */
export function textPages(value: string, columns = 22, linesPerPage = 3): string[] {
  const expanded = value.split(/\s+/).flatMap((word) => {
    const parts: string[] = [];
    for (let index = 0; index < word.length; index += columns) parts.push(word.slice(index, index + columns));
    return parts;
  }).join(" ");
  const lines = wrapLabel(expanded, columns, Number.MAX_SAFE_INTEGER);
  const pages: string[] = [];
  for (let index = 0; index < lines.length; index += linesPerPage) pages.push(lines.slice(index, index + linesPerPage).join(" "));
  return pages;
}

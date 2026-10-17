import type { Agent, AttentionItem, Project, Run, Task, TaskStatus } from "../api/types";
import { PORTFOLIO_PAGE_SIZE, portfolioRegions, projectRegion, sourceCoverageCurrent } from "./data";
import type { PortfolioEntry, PortfolioRegion } from "./data";

export type Point3 = [number, number, number];

export const stateColors: Record<TaskStatus, string> = {
  inbox: "#94a5bd", ready: "#9ebfff", working: "#77e5d2", review: "#c7a4ff",
  completed: "#86cfa5", archived: "#66778c", blocked: "#ffb46e", cancelled: "#758498",
  failed: "#ff7e88", reopened: "#e5c782",
};

export interface SceneNode {
  id: string;
  kind: "project" | "task" | "region" | "agent";
  title: string;
  subtitle: string;
  position: Point3;
  color: string;
  active: boolean;
  attention: number;
  region?: PortfolioRegion;
  action?: "expand" | "collapse" | "next" | "previous";
}

export interface SceneEdge {
  id: string;
  from: string;
  to: string;
  blocked: boolean;
  relationship?: "ownership";
}

export interface WorkspaceScene {
  nodes: SceneNode[];
  edges: SceneEdge[];
  cycleAffectedTaskIds: string[];
  unresolvedDependencies: number;
  radius: number;
  regionCounts?: Record<PortfolioRegion, number>;
  pageCount?: number;
  page?: number;
}

export interface SceneOptions { portfolio?: Record<string, PortfolioEntry>; region?: PortfolioRegion | null; page?: number; now?: number; agents?: Agent[] }

/** The same label geometry feeds rendering and the immersive information fit. */
export function nodeLabelLayout(node: SceneNode): { position: Point3; width: number } {
  if (node.kind === "region") return {
    position: [node.position[0], node.position[1] + (node.action === "expand" ? -1.15 : 0), node.position[2] + 0.2],
    width: node.action === "next" || node.action === "previous" ? 2.4 : 3.9,
  };
  return { position: [node.position[0], node.position[1] - (node.kind === "project" ? 0.85 : 0.7), node.position[2] + 0.12], width: 2.55 };
}

/** Fit recorded information into the central slot; never change the headset camera. */
export function fitImmersiveGraph(nodes: SceneNode[]): { position: Point3; scale: number } {
  if (!nodes.length) return { position: [0, -0.02, 0], scale: 0.17 };
  const bounds = nodes.map((node) => {
    const label = nodeLabelLayout(node);
    // Covers rotated glyphs, the 0.508 selection ring and the 0.515 maximum pulse.
    const glyphRadius = 0.55;
    return {
      left: Math.min(node.position[0] - glyphRadius, label.position[0] - label.width / 2),
      right: Math.max(node.position[0] + glyphRadius, label.position[0] + label.width / 2),
      bottom: Math.min(node.position[1] - glyphRadius, label.position[1] - label.width / 6),
      top: Math.max(node.position[1] + glyphRadius, label.position[1] + label.width / 6),
    };
  });
  const left = Math.min(...bounds.map((item) => item.left));
  const right = Math.max(...bounds.map((item) => item.right));
  const bottom = Math.min(...bounds.map((item) => item.bottom));
  const top = Math.max(...bounds.map((item) => item.top));
  const scale = Math.min(0.17, 1.1 / (right - left), 1 / (top - bottom));
  return { scale, position: [-(left + right) / 2 * scale, -0.02 - (bottom + top) / 2 * scale, 0] };
}

function portfolioScene(projects: Project[], options: SceneOptions): WorkspaceScene {
  const entries = options.portfolio ?? {};
  const groups: Record<PortfolioRegion, Project[]> = { coordination: [], work: [], quiet: [], unknown: [] };
  for (const project of [...projects].sort((a, b) => a.id.localeCompare(b.id))) groups[projectRegion(entries[project.id]?.overview, options.now ?? Date.now())].push(project);
  const regionCounts = { coordination: groups.coordination.length, work: groups.work.length, quiet: groups.quiet.length, unknown: groups.unknown.length };
  const nodes: SceneNode[] = [];
  let pageCount = 1;
  let page = 0;
  if (!options.region) {
    portfolioRegions.forEach((region, index) => {
      const count = regionCounts[region.id];
      const blockers = groups[region.id].reduce((sum, project) => sum + (entries[project.id]?.overview?.coordination?.counts.blocked_tasks ?? 0), 0);
      nodes.push({ id: `region:${region.id}`, kind: "region", title: `${region.badge} ${region.title}`, subtitle: `${count} projects${blockers ? ` · ${blockers} blockers` : ""} · unfold`,
        position: [index % 2 === 0 ? -3.2 : 3.2, index < 2 ? 1.7 : -1.7, 0], color: region.color, active: false, attention: blockers, region: region.id, action: "expand" });
    });
  } else {
    const region = portfolioRegions.find((item) => item.id === options.region)!;
    const members = groups[region.id];
    pageCount = Math.max(1, Math.ceil(members.length / PORTFOLIO_PAGE_SIZE));
    page = Math.min(Math.max(0, options.page ?? 0), pageCount - 1);
    nodes.push({ id: `region:${region.id}`, kind: "region", title: `${region.badge} ${region.title}`, subtitle: `${members.length} projects · page ${page + 1}/${pageCount} · all regions`, position: [0, 2.8, 0], color: region.color, active: false, attention: 0, action: "collapse" });
    members.slice(page * PORTFOLIO_PAGE_SIZE, (page + 1) * PORTFOLIO_PAGE_SIZE).forEach((project, index) => {
      const counts = entries[project.id]?.overview?.coordination?.counts;
      const blockers = counts?.blocked_tasks ?? 0;
      const requests = counts?.requests ?? 0;
      const covered = sourceCoverageCurrent(entries[project.id]?.overview?.sources, options.now ?? Date.now());
      nodes.push({ id: project.id, kind: "project", title: project.name,
        subtitle: (blockers ? `${blockers} blockers` : requests ? `${requests} requests` : region.id === "unknown" ? "Coverage unknown" : counts?.unfinished_tasks ? `${counts.unfinished_tasks} unfinished` : counts?.source_unfinished_tasks ? `${counts.source_unfinished_tasks} source plans` : "Inspect project") + (!covered && region.id !== "unknown" ? " · coverage unknown" : ""),
        position: [(index % 3 - 1) * 3.1, 1.6 - Math.floor(index / 3) * 1.6, 0], color: region.color,
        active: (counts?.running_work ?? 0) > 0, attention: blockers + requests, region: region.id });
    });
    if (page > 0) nodes.push({ id: "region:previous", kind: "region", title: "Previous projects", subtitle: `Page ${page}/${pageCount}`, position: [-2.5, -5.2, 0], color: region.color, active: false, attention: 0, action: "previous" });
    if (page + 1 < pageCount) nodes.push({ id: "region:next", kind: "region", title: "Next projects", subtitle: `Page ${page + 2}/${pageCount}`, position: [2.5, -5.2, 0], color: region.color, active: false, attention: 0, action: "next" });
  }
  return { nodes, edges: [], cycleAffectedTaskIds: [], unresolvedDependencies: 0, radius: 8, regionCounts, pageCount, page };
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
  options?: SceneOptions,
): WorkspaceScene {
  const openAttention = attention.filter((item) => item.status === "open");
  const project = projects.find((candidate) => candidate.id === activeProjectId);
  if (!project) {
    if (options?.portfolio) return portfolioScene(projects, options);
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
  const participants = (options?.agents ?? []).filter((agent) => agent.project_id === project.id);
  participants.forEach((agent, index) => {
    nodes.push({ id: agent.id, kind: "agent", title: agent.display_name, subtitle: `Reported ${agent.status} · ${agent.source.system}`,
      position: [firstColumnX - 3.05, (index + 1) * 1.4, 0], color: "#94e0e8", active: runs.some((run) => run.project_id === project.id && run.agent_id === agent.id && isWorking(run)), attention: 0 });
  });
  for (const task of projectTasks) {
    if (task.owner_id && participants.some((agent) => agent.id === task.owner_id)) edges.push({ id: `owner:${task.owner_id}:${task.id}`, from: task.owner_id, to: task.id, blocked: false, relationship: "ownership" });
  }
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
  const score = (node: SceneNode) => (node.id === selected ? 1_000_000 : 0) + (node.id === hovered ? 100_000 : 0) + (node.kind === "agent" ? 50_000 : 0) + Math.min(node.attention, 9999);
  return nodes.filter((node) => node.id === selected || node.id === hovered || node.attention > 0 || node.kind === "agent")
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

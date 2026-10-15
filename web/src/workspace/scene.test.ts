import { describe, expect, it } from "vitest";
import type { Project, Task } from "../api/types";
import { attention, run, task } from "../test/fixtures";
import { buildWorkspaceScene, identityHash, projectPosition, shortLabel, textPages, visibleLabels, wrapLabel } from "./scene";

const project: Project = { id: task.project_id, name: "Observatory", version: 1, updated_at: task.updated_at, capabilities: [] };
const second: Project = { ...project, id: "project_beta", name: "Another project" };
const makeTask = (id: string, dependencies: string[] = []): Task => ({ ...task, id, dependencies });

describe("workspace projection", () => {
  it("keeps project geography stable across insertion and input ordering", () => {
    const before = buildWorkspaceScene([project], null, [], [], []);
    const after = buildWorkspaceScene([second, project], null, [], [], []);
    expect(after.nodes.find((node) => node.id === project.id)?.position).toEqual(before.nodes[0]?.position);
    expect(buildWorkspaceScene([project, second], null, [], [], [])).toEqual(after);
    expect(projectPosition(project.id)).not.toEqual(projectPosition(second.id));
    expect(identityHash(project.id)).toBe(identityHash(project.id));
  });

  it("shows attention and running state only from actual authorized project records", () => {
    const scene = buildWorkspaceScene([project, second], null, [], [run, { ...run, id: "paused", project_id: second.id, status: "paused" }], [attention, { ...attention, id: "resolved", project_id: second.id, status: "resolved" }]);
    expect(scene.nodes.find((node) => node.id === project.id)).toMatchObject({ attention: 1, active: true, subtitle: "1 open attention", color: "#ffb46e" });
    expect(scene.nodes.find((node) => node.id === second.id)).toMatchObject({ attention: 0, active: false, subtitle: "Open project" });
    expect(scene.edges).toEqual([]);
  });

  it("uses only the selected project's tasks and declared dependency edges", () => {
    const tasks = [makeTask("A"), makeTask("B", ["A", "A", "external"]), makeTask("C", ["B"]), { ...makeTask("other"), project_id: second.id }];
    const scene = buildWorkspaceScene([project, second], project.id, tasks, [run], [attention]);
    expect(scene.nodes.map((node) => node.id)).toEqual([project.id, "A", "B", "C"]);
    expect(scene.edges.map((edge) => edge.id)).toEqual(["A:B", "B:C"]);
    expect(scene.unresolvedDependencies).toBe(1);
    const a = scene.nodes.find((node) => node.id === "A")!;
    const b = scene.nodes.find((node) => node.id === "B")!;
    const c = scene.nodes.find((node) => node.id === "C")!;
    expect(a.position[0]).toBeLessThan(b.position[0]);
    expect(b.position[0]).toBeLessThan(c.position[0]);
    expect(scene.nodes[0]?.subtitle).toBe("3 tasks");
  });

  it("preserves cycle-affected tasks instead of hiding invalid graphs", () => {
    const scene = buildWorkspaceScene([project], project.id, [makeTask("A", ["B"]), makeTask("B", ["A"]), makeTask("downstream", ["A"]), makeTask("independent")], [], []);
    expect(scene.cycleAffectedTaskIds).toEqual(["A", "B", "downstream"]);
    expect(scene.nodes).toHaveLength(5);
    expect(scene.edges).toHaveLength(3);
    expect(scene.nodes.every((node) => node.position.every(Number.isFinite))).toBe(true);
  });

  it("marks blockage at the source dependency and active pulses at the task actually running", () => {
    const a = { ...makeTask(task.id), status: "blocked" as const };
    const b = makeTask("dependent", [task.id]);
    const scene = buildWorkspaceScene([project], project.id, [a, b], [{ ...run, status: "running" }], [attention]);
    expect(scene.edges[0]?.blocked).toBe(true);
    expect(scene.nodes.find((node) => node.id === task.id)).toMatchObject({ active: true, attention: 1, color: "#ffb46e", subtitle: "blocked" });
    expect(scene.nodes.find((node) => node.id === b.id)?.active).toBe(false);
  });

  it("distinguishes source-reported blockage and completion from supervised task state", () => {
    const blocked = { ...makeTask("blocked-source"), status: "inbox" as const, source_status: "Blocked" };
    const done = { ...makeTask("done-source", [blocked.id]), status: "inbox" as const, source_status: "Done" };
    const scene = buildWorkspaceScene([project], project.id, [blocked, done], [], []);
    expect(scene.nodes.find((node) => node.id === blocked.id)).toMatchObject({ subtitle: "inbox · source Blocked", color: "#ffb46e" });
    expect(scene.nodes.find((node) => node.id === done.id)).toMatchObject({ subtitle: "inbox · source Done", color: "#94a5bd" });
    expect(scene.edges[0]?.blocked).toBe(true);
  });

  it("handles empty, removed selection, self-cycle, and wide task layers without fabricated nodes", () => {
    expect(buildWorkspaceScene([], null, [], [], []).nodes).toEqual([]);
    expect(buildWorkspaceScene([project], "removed", [], [], []).nodes[0]?.kind).toBe("project");
    expect(buildWorkspaceScene([project], project.id, [], [], []).nodes).toHaveLength(1);
    expect(buildWorkspaceScene([project], project.id, [makeTask("self", ["self"])], [], []).cycleAffectedTaskIds).toEqual(["self"]);
    const wide = buildWorkspaceScene([project], project.id, Array.from({ length: 22 }, (_, index) => makeTask(`wide-${index}`)), [], []);
    expect(wide.nodes).toHaveLength(23);
    expect(new Set(wide.nodes.map((node) => JSON.stringify(node.position))).size).toBe(23);
    expect(wide.radius).toBeGreaterThanOrEqual(5);
  });

  it("separates wrapped task labels, centers partial columns, and reserves width before the next dependency layer", () => {
    const tasks = Array.from({ length: 8 }, (_, index) => makeTask(`parallel-${index}`));
    const scene = buildWorkspaceScene([project], project.id, [...tasks, makeTask("following-layer", [tasks[0]!.id])], [], []);
    const parallel = scene.nodes.filter((node) => node.id.startsWith("parallel-"));
    for (let first = 0; first < parallel.length; first += 1) {
      for (let second = first + 1; second < parallel.length; second += 1) {
        const a = parallel[first]!.position;
        const b = parallel[second]!.position;
        expect(Math.abs(a[0] - b[0]) >= 2.15 || Math.abs(a[1] - b[1]) >= 0.72).toBe(true);
      }
    }
    const lastColumnX = Math.max(...parallel.map((node) => node.position[0]));
    const lastColumn = parallel.filter((node) => node.position[0] === lastColumnX);
    expect(lastColumn).toHaveLength(3);
    expect(lastColumn.reduce((sum, node) => sum + node.position[1], 0)).toBe(0);
    const following = scene.nodes.find((node) => node.id === "following-layer")!;
    expect(following.position[0] - lastColumnX).toBeGreaterThanOrEqual(2.7);
  });

  it("does not mutate API entities while projecting their current state", () => {
    const tasks = [makeTask("B", ["A"]), makeTask("A")];
    const snapshot = JSON.stringify(tasks);
    buildWorkspaceScene([project], project.id, tasks, [], []);
    expect(JSON.stringify(tasks)).toBe(snapshot);
  });
});

describe("readable canvas labels", () => {
  it("wraps words and bounds long names while preserving ordinary names", () => {
    expect(shortLabel("Small")).toBe("Small");
    expect(shortLabel("Longer name", 7)).toBe("Longer…");
    expect(wrapLabel("One two three four five six", 9, 2)).toEqual(["One two", "three…"]);
    expect(wrapLabel("a".repeat(80), 10)).toEqual(["aaaaaaaaa…"]);
    expect(wrapLabel("   ")).toEqual(["Untitled"]);
  });

  it("paginates complete evidence including long URLs without dropping content", () => {
    const original = `This is the complete evidence summary with ${"x".repeat(100)} preserved across pages.`;
    const pages = textPages(original);
    expect(pages.length).toBeGreaterThan(1);
    expect(pages.join("").replaceAll(" ", "")).toBe(original.replaceAll(" ", ""));
    expect(textPages("")).toEqual(["Untitled"]);
  });

  it("bounds dense portfolio labels while keeping selection, hover, and highest attention readable", () => {
    const nodes = Array.from({ length: 97 }, (_, index) => ({
      id: `project-${index}`, kind: "project" as const, title: `Project ${index}`, subtitle: "Open project",
      position: projectPosition(`project-${index}`), color: "#fff", active: false, attention: index,
    }));
    const labels = visibleLabels(nodes, "project-0", "project-1");
    expect(labels).toHaveLength(8);
    expect(labels.slice(0, 3).map((node) => node.id)).toEqual(["project-0", "project-1", "project-96"]);
    expect(visibleLabels(nodes.slice(0, 3), null, null)).toHaveLength(3);
    expect(nodes[0]?.id).toBe("project-0");
  });
});

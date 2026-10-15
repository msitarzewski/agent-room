import { Component, Suspense, lazy, useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { api } from "../api/client";
import type { Claim, Task } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { ActionFeedback, RelativeTime, StatusPill, hasCapability } from "../components/Common";
import { useStream } from "../stream/StreamContext";
import { loadPortfolio, type PortfolioEntry } from "../workspace/data";
import { useWorkspace } from "../workspace/useWorkspace";

const SceneCanvas = lazy(() => import("../workspace/SceneCanvas").then(({ SceneCanvas }) => ({ default: SceneCanvas })));

class SceneBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  render() {
    return this.state.failed ? <div className="workspace-scene__fallback" role="status">The spatial view is unavailable. Your projects, tasks, and evidence remain available below.</div> : this.props.children;
  }
}

function useReducedMotion() {
  const [reduced, setReduced] = useState(() => window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false);
  useEffect(() => {
    const media = window.matchMedia?.("(prefers-reduced-motion: reduce)");
    if (!media) return;
    const update = () => setReduced(media.matches);
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);
  return reduced;
}

function updateAddress(projectId: string | null, taskId: string | null) {
  const url = new URL(window.location.href);
  if (projectId) url.searchParams.set("project", projectId); else url.searchParams.delete("project");
  if (taskId) url.searchParams.set("task", taskId); else url.searchParams.delete("task");
  window.history.replaceState({}, "", url);
}

export function WorkspacePage() {
  const { projects, projectId, setProjectId } = useAuth();
  const stream = useStream();
  const reducedMotion = useReducedMotion();
  const [focusId, setFocusId] = useState<string | null>(() => new URLSearchParams(window.location.search).get("project"));
  const [taskId, setTaskId] = useState<string | null>(() => new URLSearchParams(window.location.search).get("task"));
  const focusedProject = projects.find((project) => project.id === focusId) ?? null;
  const contextId = focusedProject?.id ?? projectId;
  const workspace = useWorkspace(contextId);
  const snapshot = workspace.snapshot?.project_id === contextId ? workspace.snapshot : null;
  const resources = snapshot?.resources;
  const selected = focusedProject ? resources?.tasks.find((task) => task.id === taskId) : null;
  const [directory, setDirectory] = useState<Record<string, PortfolioEntry>>({});
  const [directoryError, setDirectoryError] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [review, setReview] = useState<{ claim: Claim; decision: "accepted" | "rejected" } | null>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const initialized = useRef(false);
  const previousProject = useRef(projectId);
  const actionGeneration = useRef(0);
  const capabilities = projects.find((project) => project.id === contextId)?.capabilities ?? [];

  useEffect(() => {
    let controller = new AbortController();
    const refresh = () => {
      controller.abort();
      controller = new AbortController();
      const request = controller;
      void loadPortfolio(projects, request.signal).then((next) => {
        if (!request.signal.aborted) { setDirectory(next); setDirectoryError(null); }
      }).catch((caught: unknown) => {
        if (!request.signal.aborted) setDirectoryError(caught instanceof Error ? caught.message : "Project summaries are unavailable.");
      });
    };
    refresh();
    const interval = window.setInterval(refresh, 60_000);
    return () => { controller.abort(); window.clearInterval(interval); };
  }, [projects]);

  useEffect(() => {
    if (!initialized.current) {
      initialized.current = true;
      if (focusedProject && focusedProject.id !== projectId) setProjectId(focusedProject.id);
    } else if (previousProject.current !== projectId && focusedProject && focusedProject.id !== projectId) {
      setFocusId(projectId);
      setTaskId(null);
      updateAddress(projectId, null);
    }
    previousProject.current = projectId;
  }, [focusedProject, projectId, setProjectId]);

  useEffect(() => {
    setReview(null);
    setBusy(false);
    setMessage(null);
    setActionError(null);
    return () => { actionGeneration.current += 1; };
  }, [contextId, taskId]);

  const selectProject = useCallback((id: string) => {
    if (!projects.some((project) => project.id === id)) return;
    setProjectId(id);
    setFocusId(id);
    setTaskId(null);
    setQuery("");
    updateAddress(id, null);
  }, [projects, setProjectId]);

  const selectTask = useCallback((id: string) => {
    setTaskId(id);
    updateAddress(focusId, id);
    heading.current?.focus();
  }, [focusId]);

  const back = useCallback(() => {
    setFocusId(null);
    setTaskId(null);
    setQuery("");
    updateAddress(null, null);
  }, []);

  const perform = async (operation: () => Promise<unknown>, success: string) => {
    const generation = actionGeneration.current;
    setBusy(true);
    setActionError(null);
    setMessage(null);
    try {
      await operation();
      if (generation !== actionGeneration.current) return;
      setMessage(success);
      await workspace.refresh();
    } catch (caught) {
      if (generation !== actionGeneration.current) return;
      setActionError(caught instanceof Error ? caught.message : "The action could not be completed.");
    } finally { if (generation === actionGeneration.current) setBusy(false); }
  };

  const normalizedQuery = query.trim().toLowerCase();
  const visibleProjects = projects.filter((project) => project.name.toLowerCase().includes(normalizedQuery));
  const tasks = focusedProject && resources ? resources.tasks : [];
  const visibleTasks = tasks.filter((task) => `${task.title} ${task.status}`.toLowerCase().includes(normalizedQuery));
  const attention = resources?.attention.filter((item) => item.status !== "resolved") ?? [];
  const taskRuns = resources?.runs.filter((run) => run.task_id === selected?.id) ?? [];
  const evidence = resources?.evidence.filter((item) => selected && (item.task_id === selected.id || taskRuns.some((run) => run.id === item.run_id))) ?? [];
  const artifacts = resources?.artifacts.filter((item) => selected && (item.task_id === selected.id || taskRuns.some((run) => run.id === item.run_id))) ?? [];
  const claim = resources?.claims.find((item) => item.task_id === selected?.id && item.status === "pending");

  const chooseAttention = (resourceId?: string) => {
    if (!contextId) return;
    selectProject(contextId);
    if (resourceId && resources?.tasks.some((task) => task.id === resourceId)) {
      setTaskId(resourceId);
      updateAddress(contextId, resourceId);
    }
  };

  return (
    <div className="observatory">
      <header className="observatory-header">
        <div><p className="eyebrow">A place for everything in motion</p><h1>Your observatory</h1><p>Follow the work. Find the decision. See the evidence.</p></div>
        <div className="observatory-signal"><span className="observatory-signal__orb" aria-hidden="true" /><span>{projects.length} authorized {projects.length === 1 ? "project" : "projects"}<small>{stream.status === "connected" ? "Connected to the current project" : "Live connection " + stream.status}</small></span></div>
      </header>
      <nav className="observatory-trail" aria-label="Workspace location">
        <button type="button" onClick={back} aria-current={!focusedProject ? "location" : undefined}>Portfolio</button>
        {focusedProject ? <><span aria-hidden="true">/</span><button type="button" onClick={() => { setTaskId(null); updateAddress(focusedProject.id, null); }} aria-current={!selected ? "location" : undefined}>{focusedProject.name}</button></> : null}
        {selected ? <><span aria-hidden="true">/</span><span aria-current="location">{selected.title}</span></> : null}
      </nav>

      <section className="observatory-attention" aria-labelledby="workspace-attention-title"><div><p className="eyebrow">Within reach</p><h2 id="workspace-attention-title">Attention <span>{attention.length}</span></h2><small>{projects.find((project) => project.id === contextId)?.name ?? "Select a project"}</small></div><div className="observatory-attention__items">{attention.map((item) => <article key={item.id}><button type="button" onClick={() => chooseAttention(item.resource_id)}><span className={`observatory-attention__dot observatory-attention__dot--${item.severity}`} aria-hidden="true" /><strong>{item.title}</strong><small>{item.severity} · {item.status}</small></button>{item.status === "open" && hasCapability(capabilities, "attention:manage") ? <button className="text-button" disabled={busy || workspace.stale} onClick={() => void perform(() => api.attentionAction(item, "acknowledge"), "Attention acknowledged.")}>Acknowledge</button> : null}</article>)}{attention.length === 0 ? <p>{snapshot ? "Nothing waiting for you in this project." : "Project attention is loading."}</p> : null}</div></section>

      <div className="observatory-body">
        <section className="observatory-world" aria-label="Spatial workspace">
          <SceneBoundary><Suspense fallback={<div className="workspace-scene__fallback" role="status">Opening your spatial workspace…</div>}>
            <SceneCanvas projects={projects} activeProjectId={focusedProject?.id ?? null} tasks={tasks} runs={resources?.runs ?? []} attention={attention} evidence={resources?.evidence ?? []} artifacts={resources?.artifacts ?? []} selectedTaskId={selected?.id ?? null} onProjectSelect={selectProject} onTaskSelect={selectTask} onBack={back} reducedMotion={reducedMotion} />
          </Suspense></SceneBoundary>
          <div className="observatory-world__caption"><span>{focusedProject ? "Every connection is a recorded dependency" : "Choose a project to unfold its work"}</span>{reducedMotion ? <span>Reduced motion</span> : null}</div>
        </section>

        <aside className="observatory-inspector" aria-label="Workspace inspector">
          <p className="eyebrow">{selected ? "Task / evidence" : focusedProject ? "Project / work" : "Portfolio / overview"}</p>
          <h2 ref={heading} tabIndex={-1}>{selected?.title ?? focusedProject?.name ?? "A little perspective."}</h2>
          {!focusedProject ? <p className="observatory-muted">Your projects have a place. Open one to see its work, dependencies, and the decisions waiting for you.</p> : null}
          {focusedProject && !snapshot && workspace.state === "loading" ? <p role="status">Reading project state…</p> : null}
          {workspace.state === "denied" ? <p role="alert">Your role cannot inspect this project's complete workspace.</p> : null}
          {workspace.state === "error" ? <div role="alert"><p>{workspace.error}</p><button className="button button--secondary" onClick={() => void workspace.refresh()}>Retry project</button></div> : null}
          {workspace.stale && snapshot ? <p className="observatory-warning" role="status">Project data may be stale. Refresh before acting.</p> : null}
          {selected ? <>
            <div className="observatory-pills"><StatusPill value={selected.status} /><StatusPill value={selected.priority} /></div>
            {selected.description ? <p>{selected.description}</p> : null}
            {selected.blocked_reason ? <p className="observatory-warning">{selected.blocked_reason}</p> : null}
            <dl className="observatory-facts"><div><dt>Owner</dt><dd>{resources?.agents.find((agent) => agent.id === selected.owner_id)?.display_name ?? selected.owner_id ?? "Unassigned"}</dd></div><div><dt>Source</dt><dd>{selected.source.system}</dd></div><div><dt>Review</dt><dd>{selected.review_state ?? "not requested"}</dd></div><div><dt>Approval</dt><dd>{selected.approval_state ?? "not required"}</dd></div></dl>
            {"source_status" in selected && typeof selected.source_status === "string" ? <p className="observatory-muted">Source reports: {selected.source_status}. Supervised state: {selected.status}.</p> : null}
            {selected.source_observation ? <section aria-label="Latest source report"><h3>Latest source report</h3><article className="observatory-evidence">
              <strong>{selected.source_observation.title}</strong>
              {selected.source_observation.description ? <p style={{ whiteSpace: "pre-wrap" }}>{selected.source_observation.description}</p> : null}
              <small>Reported status: {selected.source_observation.status} · {selected.source.system}</small>
              {selected.source_observed_at ? <small>Observed <RelativeTime value={selected.source_observed_at} /></small> : null}
              <p>Source reference: <code>{selected.source_observation.external_url ?? selected.source_observation.external_id}</code></p>
            </article></section> : null}
            {selected.dependencies.length ? <section><h3>Depends on</h3><ul className="observatory-links">{selected.dependencies.map((id) => <li key={id}>{tasks.some((task) => task.id === id) ? <button type="button" onClick={() => selectTask(id)}>{tasks.find((task) => task.id === id)?.title}</button> : <span>{id} · details unavailable</span>}</li>)}</ul></section> : null}
            <section><h3>Evidence <span>{evidence.length}</span></h3>{evidence.length ? evidence.map((item) => <article className="observatory-evidence" key={item.id}><strong>{item.summary}</strong><small>{item.kind} · {item.source.system}</small>{item.digest ? <code>{item.digest}</code> : null}</article>) : <p className="observatory-muted">No evidence linked to this task.</p>}</section>
            {artifacts.length ? <section><h3>Artifacts</h3>{artifacts.map((item) => <a className="observatory-artifact" key={item.id} href={`/api/v1/artifacts/${encodeURIComponent(item.id)}/content?${new URLSearchParams({ project_id: item.project_id })}`}>{item.name}<small>{item.media_type}</small></a>)}</section> : null}
            {taskRuns.length ? <section><h3>Runs</h3>{taskRuns.map((run) => <article className="observatory-evidence" key={run.id}><strong>{run.summary ?? run.id}</strong><StatusPill value={run.status} />{run.status === "running" || run.status === "working" ? hasCapability(capabilities, "run:pause") && run.capabilities.includes("run:pause") ? <button className="button button--secondary" disabled={busy || workspace.stale} onClick={() => void perform(() => api.runAction(run, "pause"), "Pause requested.")}>Pause run</button> : null : null}</article>)}</section> : null}
            {selected.status === "review" && claim && hasCapability(capabilities, "claim:review") ? <div className="observatory-review"><button className="button button--primary" disabled={busy || workspace.stale} onClick={() => setReview({ claim, decision: "accepted" })}>Review completion</button><button className="button button--secondary" disabled={busy || workspace.stale} onClick={() => setReview({ claim, decision: "rejected" })}>Reject claim</button></div> : null}
          </> : <div className="observatory-metrics"><div><strong>{focusedProject ? tasks.length : projects.length}</strong><span>{focusedProject ? "tasks" : "projects"}</span></div><div><strong>{snapshot ? attention.length : "—"}</strong><span>need attention in {projects.find((project) => project.id === contextId)?.name ?? "selected project"}</span></div></div>}
          {snapshot ? <section className="observatory-sources"><h3>Source observations</h3>{snapshot.sources.length ? snapshot.sources.map((source) => <div key={source.system}><span>{source.system}</span><small>last received <RelativeTime value={source.last_seen} /></small></div>) : <p className="observatory-muted">No adapter observations received.</p>}<small>Snapshot <RelativeTime value={snapshot.generated_at} /> · event {snapshot.cursor}</small></section> : null}
          <ActionFeedback message={message} error={actionError} />
          {review && selected ? <div className="observatory-confirm" role="alertdialog" aria-labelledby="workspace-review-title"><h3 id="workspace-review-title">{review.decision === "accepted" ? "Accept completion claim?" : "Reject completion claim?"}</h3><p>{selected.title}. Inspect the linked evidence before recording your decision.</p><button className="button button--primary" disabled={busy || workspace.stale} onClick={() => { const decision = review; setReview(null); void perform(() => api.reviewClaim(decision.claim, decision.decision), "Review decision recorded."); }}>Confirm decision</button><button className="button button--secondary" onClick={() => setReview(null)}>Keep reviewing</button></div> : null}
        </aside>
      </div>

      <section className="observatory-directory" aria-labelledby="workspace-directory-title">
        {directoryError ? <p className="observatory-warning" role="status">{directoryError}</p> : null}
        <div className="observatory-directory__header"><h2 id="workspace-directory-title">{focusedProject ? "Work in this project" : "Your project directory"}</h2><label><span className="sr-only">{focusedProject ? "Search tasks" : "Search projects"}</span><input type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder={focusedProject ? "Find a task…" : "Find a project…"} /></label></div>
        <div className="observatory-directory__items" role="list">
          {focusedProject ? visibleTasks.map((task: Task) => <div role="listitem" key={task.id}><button className={`observatory-card ${selected?.id === task.id ? "is-selected" : ""}`} type="button" onClick={() => selectTask(task.id)}><span className={`observatory-card__mark observatory-card__mark--${task.status}`} aria-hidden="true" /><strong>{task.title}</strong><span>{task.status} · {task.dependencies.length} dependencies</span></button></div>) : visibleProjects.map((project) => <div role="listitem" key={project.id}><button className="observatory-card" type="button" onClick={() => selectProject(project.id)}><span className="observatory-card__mark" aria-hidden="true" /><strong>{project.name}</strong><span>{directory[project.id]?.overview ? `${directory[project.id]?.overview?.counts.task ?? 0} tasks · ${directory[project.id]?.overview?.counts.attention ?? 0} attention records` : directory[project.id]?.state === "denied" ? "Summary access restricted" : directory[project.id]?.state === "error" ? "Summary unavailable" : "Reading summary…"}</span></button></div>)}
        </div>
        {(focusedProject ? visibleTasks.length : visibleProjects.length) === 0 ? <p className="observatory-muted">{query ? "Nothing matches your search." : focusedProject ? "No tasks have been recorded for this project." : "No projects are available to your account."}</p> : null}
      </section>


    </div>
  );
}

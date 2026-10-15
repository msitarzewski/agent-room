import { Component, Suspense, lazy, useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { api } from "../api/client";
import type { Claim, CoordinationRequest, Task } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { ActionFeedback, RelativeTime, SourceFreshness, StatusPill, hasCapability } from "../components/Common";
import { useStream } from "../stream/StreamContext";
import { loadPortfolio, PORTFOLIO_PAGE_SIZE, portfolioRegions, portfolioSignals, projectRegion, type PortfolioEntry, type PortfolioRegion, type PortfolioSignal } from "../workspace/data";
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
  const { projects, projectId, setProjectId, session } = useAuth();
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
  const [region, setRegion] = useState<PortfolioRegion | null>(null);
  const [regionPage, setRegionPage] = useState(0);
  const [now, setNow] = useState(Date.now);
  const [requestId, setRequestId] = useState<string | null>(null);
  const [responseText, setResponseText] = useState("");
  const [requestAction, setRequestAction] = useState<{ action: "respond" | "resolve" | "cancel"; request: CoordinationRequest; body: string } | null>(null);
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
    const timer = window.setInterval(() => setNow(Date.now()), 60_000);
    return () => window.clearInterval(timer);
  }, []);

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
      setRequestId(null);
      updateAddress(projectId, null);
    }
    previousProject.current = projectId;
  }, [focusedProject, projectId, setProjectId]);

  useEffect(() => {
    setReview(null);
    setRequestAction(null);
    setResponseText("");
    setBusy(false);
    setMessage(null);
    setActionError(null);
    return () => { actionGeneration.current += 1; };
  }, [contextId, taskId, requestId]);

  const selectProject = useCallback((id: string) => {
    if (!projects.some((project) => project.id === id)) return;
    setProjectId(id);
    setFocusId(id);
    setTaskId(null);
    setRequestId(null);
    setQuery("");
    updateAddress(id, null);
  }, [projects, setProjectId]);

  const selectTask = useCallback((id: string) => {
    setTaskId(id);
    setRequestId(null);
    updateAddress(focusId, id);
    heading.current?.focus();
  }, [focusId]);

  const back = useCallback(() => {
    setFocusId(null);
    setTaskId(null);
    setRequestId(null);
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
  const matchingProjects = projects.filter((project) => project.name.toLowerCase().includes(normalizedQuery) && (!region || projectRegion(directory[project.id]?.overview, now) === region)).sort((a, b) => a.id.localeCompare(b.id));
  const pageCount = Math.max(1, Math.ceil(matchingProjects.length / PORTFOLIO_PAGE_SIZE));
  const displayedPage = Math.min(regionPage, pageCount - 1);
  const visibleProjects = !normalizedQuery && !region && projects.length > PORTFOLIO_PAGE_SIZE ? [] : matchingProjects.slice(displayedPage * PORTFOLIO_PAGE_SIZE, (displayedPage + 1) * PORTFOLIO_PAGE_SIZE);
  const signals = portfolioSignals(projects, directory);
  const coordination = signals.filter((item) => !item.human_escalation);
  const escalations = signals.filter((item) => item.human_escalation);
  const summariesIncomplete = projects.some((project) => !directory[project.id]?.overview?.coordination?.complete);
  const tasks = focusedProject && resources ? resources.tasks : [];
  const visibleTasks = tasks.filter((task) => `${task.title} ${task.status}`.toLowerCase().includes(normalizedQuery));
  const attention = resources?.attention.filter((item) => item.status !== "resolved") ?? [];
  const taskRuns = resources?.runs.filter((run) => run.task_id === selected?.id) ?? [];
  const evidence = resources?.evidence.filter((item) => selected && (item.task_id === selected.id || taskRuns.some((run) => run.id === item.run_id))) ?? [];
  const artifacts = resources?.artifacts.filter((item) => selected && (item.task_id === selected.id || taskRuns.some((run) => run.id === item.run_id))) ?? [];
  const claim = resources?.claims.find((item) => item.task_id === selected?.id && item.status === "pending");
  const requests = resources?.requests ?? [];
  const selectedRequest = focusedProject ? requests.find((item) => item.id === requestId) : undefined;
  const actorName = (id: string) => resources?.agents.find((item) => item.id === id)?.display_name ?? (session?.user.id === id ? session.user.display_name : undefined) ?? signals.find((item) => item.owner_id === id)?.owner_name ?? signals.find((item) => item.recipient?.id === id)?.recipient_name ?? id;
  const operator = hasCapability(capabilities, "resource:write");
  const mayRespond = selectedRequest && selectedRequest.status === "open" && (operator || selectedRequest.recipient.type === "human" && selectedRequest.recipient.id === session?.user.id && hasCapability(capabilities, "coordination:respond"));
  const mayClose = (action: "resolve" | "cancel") => selectedRequest && ["open", "responded"].includes(selectedRequest.status) && (operator || selectedRequest.owner_type === "human" && selectedRequest.owner_id === session?.user.id && hasCapability(capabilities, `coordination:${action}`));
  const reviewRequest = (action: "respond" | "resolve" | "cancel") => {
    if (selectedRequest) setRequestAction({ action, request: selectedRequest, body: responseText.trim() });
  };
  const chooseRegion = (next: PortfolioRegion | null) => { setRegion(next); setRegionPage(0); setQuery(""); };
  const inspectSignal = (signal: PortfolioSignal) => {
    selectProject(signal.project_id);
    if (signal.resource_type === "coordination_request") setRequestId(signal.id);
    else if (signal.task_id || signal.resource_type === "task" || signal.target_type === "task") {
      const id = signal.task_id ?? signal.target_id ?? signal.id;
      setTaskId(id);
      updateAddress(signal.project_id, id);
    }
  };

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
        <div><p className="eyebrow">An agent coordination room</p><h1>Your observatory</h1><p>Follow who is working, what they need, and the evidence behind it.</p></div>
        <div className="observatory-signal"><span className="observatory-signal__orb" aria-hidden="true" /><span>{projects.length} authorized {projects.length === 1 ? "project" : "projects"}<small>{stream.status === "connected" ? "Connected to the current project" : "Live connection " + stream.status}</small></span></div>
      </header>
      <nav className="observatory-trail" aria-label="Workspace location">
        <button type="button" onClick={back} aria-current={!focusedProject ? "location" : undefined}>Portfolio</button>
        {focusedProject ? <><span aria-hidden="true">/</span><button type="button" onClick={() => { setTaskId(null); updateAddress(focusedProject.id, null); }} aria-current={!selected ? "location" : undefined}>{focusedProject.name}</button></> : null}
        {selected ? <><span aria-hidden="true">/</span><span aria-current="location">{selected.title}</span></> : null}
      </nav>

      <div className="observatory-coordination">
        <section aria-labelledby="coordination-heading"><p className="eyebrow">Agents / work / recorded asks</p><h2 id="coordination-heading">Coordination <span>{coordination.length}</span></h2>
          <div className="observatory-coordination__items">{coordination.slice(0, 12).map((item) => <article key={`${item.project_id}:${item.kind}:${item.id}`} className={`observatory-coordination__item is-${item.kind}`}>
            <button type="button" onClick={() => inspectSignal(item)}><small>{item.project_name} · {item.request_kind ?? item.kind} · {item.status}</small><strong>{item.owner_name ?? item.owner_id ?? "Owner not recorded"}{item.recipient ? ` → ${item.recipient_name ?? item.recipient.id} (${item.recipient.type})` : ""}</strong><span>{item.title}</span>{item.detail ? <p>{item.detail}</p> : null}</button>
            <small>{item.source_system ? `${item.source_system} · ` : ""}Recorded <RelativeTime value={item.updated_at} /></small>
          </article>)}</div>
          {!coordination.length ? <p className="observatory-muted">No agent participation or coordination request is recorded in the available summaries.</p> : null}
          {coordination.length > 12 ? <p className="observatory-muted">Showing 12 priority records. Open a project to inspect its full work and request feed.</p> : null}
          {summariesIncomplete || projects.some((project) => directory[project.id]?.overview?.coordination?.truncated) ? <small>Summary coverage is incomplete or bounded. Missing records do not establish quiet.</small> : null}
        </section>
        <section className="observatory-escalations" aria-labelledby="human-escalations-heading"><p className="eyebrow">Passenger / explicit decisions</p><h2 id="human-escalations-heading">Human escalations <span>{escalations.length}</span></h2>{escalations.slice(0, 6).map((item) => <article key={`${item.project_id}:${item.id}`}><button type="button" onClick={() => inspectSignal(item)}><strong>{item.title}</strong><small>{item.owner_name ?? item.owner_id ?? "Owner not recorded"} → {item.recipient_name ?? item.recipient?.id ?? "Human approval"}</small>{item.detail ? <p>{item.detail}</p> : null}</button></article>)}{!escalations.length ? <p>No explicit human escalation is recorded.</p> : null}</section>
      </div>

      <section className="observatory-attention" aria-labelledby="workspace-attention-title"><div><p className="eyebrow">Within reach</p><h2 id="workspace-attention-title">Attention <span>{attention.length}</span></h2><small>{projects.find((project) => project.id === contextId)?.name ?? "Select a project"}</small></div><div className="observatory-attention__items">{attention.map((item) => <article key={item.id}><button type="button" onClick={() => chooseAttention(item.resource_id)}><span className={`observatory-attention__dot observatory-attention__dot--${item.severity}`} aria-hidden="true" /><strong>{item.title}</strong><small>{item.severity} · {item.status}</small></button>{item.status === "open" && hasCapability(capabilities, "attention:manage") ? <button className="text-button" disabled={busy || workspace.stale} onClick={() => void perform(() => api.attentionAction(item, "acknowledge"), "Attention acknowledged.")}>Acknowledge</button> : null}</article>)}{attention.length === 0 ? <p>{snapshot ? "Nothing waiting for you in this project." : "Project attention is loading."}</p> : null}</div></section>

      <div className="observatory-body">
        <section className="observatory-world" aria-label="Spatial workspace">
          <SceneBoundary><Suspense fallback={<div className="workspace-scene__fallback" role="status">Opening your spatial workspace…</div>}>
            <SceneCanvas projects={projects} activeProjectId={focusedProject?.id ?? null} tasks={tasks} runs={resources?.runs ?? []} agents={resources?.agents ?? []} requests={requests} portfolio={directory} region={region} page={displayedPage} now={now} signals={signals} onRegionSelect={chooseRegion} onRegionPageChange={setRegionPage} onSignalSelect={inspectSignal} attention={attention} evidence={resources?.evidence ?? []} artifacts={resources?.artifacts ?? []} selectedTaskId={selected?.id ?? null} selectedRequestId={selectedRequest?.id ?? null} onProjectSelect={selectProject} onTaskSelect={selectTask} onBack={back} reducedMotion={reducedMotion} />
          </Suspense></SceneBoundary>
          <div className="observatory-world__caption"><span>{focusedProject ? "Connections show declared dependencies and recorded agent ownership" : "Unfold a region, then a project. Coverage unknown stays distinct from quiet."}</span>{reducedMotion ? <span>Reduced motion</span> : null}</div>
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
          {snapshot ? <section className="observatory-sources"><h3>Source observations</h3>{snapshot.sources.length ? snapshot.sources.map((source) => <SourceFreshness key={source.system} source={source} />) : <p className="observatory-muted">No adapter observations received.</p>}<small>Snapshot <RelativeTime value={snapshot.generated_at} /> · event {snapshot.cursor}</small></section> : null}
          {focusedProject && resources?.agents.length ? <section aria-label="Recorded participants"><h3>Recorded participants</h3>{resources.agents.map((item) => <article className="observatory-evidence" key={item.id}><strong>Agent · {item.display_name}</strong><span>Reported {item.status} · {item.source.system}</span><small>Observed <RelativeTime value={item.observed_at ?? item.updated_at} />{item.presence_expires_at ? <> · Presence lease ends <RelativeTime value={item.presence_expires_at} /></> : " · no live presence lease recorded"}</small></article>)}<small>Idle is availability. A source project marked Active is project context.</small></section> : null}
          <ActionFeedback message={message} error={actionError} />
          {focusedProject ? <section aria-label="Coordination requests"><h3>Coordination requests <span>{requests.filter((item) => ["open", "responded"].includes(item.status)).length}</span></h3>
            {resources?.requests === undefined ? <p className="observatory-muted">The coordination request feed is unavailable in this snapshot.</p> : !requests.length ? <p className="observatory-muted">No coordination requests have been recorded.</p> : <ul className="observatory-links">{requests.map((item) => <li key={item.id}><button type="button" aria-current={requestId === item.id ? "true" : undefined} onClick={() => { setRequestId(item.id); setTaskId(null); updateAddress(focusId, null); }}>{item.title} · {item.kind} · {item.status}</button></li>)}</ul>}
            {selectedRequest ? <article className="observatory-request" aria-label="Request details"><h4>{selectedRequest.title}</h4><StatusPill value={selectedRequest.status} /><p style={{ whiteSpace: "pre-wrap" }}>{selectedRequest.body}</p>
              <dl className="observatory-facts"><div><dt>From</dt><dd>{actorName(selectedRequest.owner_id)} ({selectedRequest.owner_type})</dd></div><div><dt>To</dt><dd>{actorName(selectedRequest.recipient.id)} ({selectedRequest.recipient.type})</dd></div><div><dt>Kind</dt><dd>{selectedRequest.kind}</dd></div></dl>
              {selectedRequest.escalation_reason ? <p>Escalation reason: {selectedRequest.escalation_reason}</p> : null}
              {selectedRequest.task_id ? <button className="text-button" type="button" onClick={() => selectTask(selectedRequest.task_id!)}>Inspect linked task</button> : null}
              {selectedRequest.evidence_ids.map((id) => <p key={id}>Evidence: {resources?.evidence.find((item) => item.id === id)?.summary ?? `${id} · details unavailable`}</p>)}
              {selectedRequest.response ? <div><strong>Recorded response · {actorName(selectedRequest.response.actor_id)}</strong><p style={{ whiteSpace: "pre-wrap" }}>{selectedRequest.response.body}</p><RelativeTime value={selectedRequest.response.responded_at} />{selectedRequest.response.evidence_ids.map((id) => <p key={id}>Response evidence: {resources?.evidence.find((item) => item.id === id)?.summary ?? `${id} · details unavailable`}</p>)}</div> : null}
              {selectedRequest.resolution ? <p>Closed by {selectedRequest.resolution.actor_id}: {selectedRequest.resolution.note}</p> : null}
              {mayRespond || mayClose("resolve") || mayClose("cancel") ? <div className="observatory-request__actions"><label>Response or closure note<textarea value={responseText} maxLength={16384} onChange={(event) => setResponseText(event.target.value)} /></label>
                {mayRespond ? <button className="button button--primary" type="button" disabled={busy || workspace.stale || !responseText.trim()} onClick={() => reviewRequest("respond")}>Review response</button> : null}
                {mayClose("resolve") ? <button className="button button--secondary" type="button" disabled={busy || workspace.stale} onClick={() => reviewRequest("resolve")}>Resolve request</button> : null}
                {mayClose("cancel") ? <button className="button button--secondary" type="button" disabled={busy || workspace.stale} onClick={() => reviewRequest("cancel")}>Cancel request</button> : null}
              </div> : <p className="observatory-muted">Only the authorized recipient, owner or project operator can respond or close this request.</p>}
              {requestAction ? <div className="observatory-confirm" role="alertdialog" aria-labelledby="request-confirm-title"><h4 id="request-confirm-title">Confirm {requestAction.action} request?</h4><p>{requestAction.request.title} · version {requestAction.request.version}. This records a coordination decision.</p>{requestAction.body ? <p style={{ whiteSpace: "pre-wrap" }}>{requestAction.body}</p> : null}<button className="button button--primary" disabled={busy || workspace.stale} onClick={() => { const decision = requestAction; setRequestAction(null); void perform(() => decision.action === "respond" ? api.respondToRequest(decision.request, decision.body) : api.closeRequest(decision.request, decision.action, decision.body), "Coordination decision recorded."); }}>Confirm coordination decision</button><button className="button button--secondary" onClick={() => setRequestAction(null)}>Keep reviewing request</button></div> : null}
            </article> : null}
          </section> : null}
          {review && selected ? <div className="observatory-confirm" role="alertdialog" aria-labelledby="workspace-review-title"><h3 id="workspace-review-title">{review.decision === "accepted" ? "Accept completion claim?" : "Reject completion claim?"}</h3><p>{selected.title}. Inspect the linked evidence before recording your decision.</p><button className="button button--primary" disabled={busy || workspace.stale} onClick={() => { const decision = review; setReview(null); void perform(() => api.reviewClaim(decision.claim, decision.decision), "Review decision recorded."); }}>Confirm decision</button><button className="button button--secondary" onClick={() => setReview(null)}>Keep reviewing</button></div> : null}
        </aside>
      </div>

      <section className="observatory-directory" aria-labelledby="workspace-directory-title">
        {directoryError ? <p className="observatory-warning" role="status">{directoryError}</p> : null}
        <div className="observatory-directory__header"><h2 id="workspace-directory-title">{focusedProject ? "Work in this project" : "Your project directory"}</h2><label><span className="sr-only">{focusedProject ? "Search tasks" : "Search projects"}</span><input type="search" value={query} onChange={(event) => { setQuery(event.target.value); setRegionPage(0); }} placeholder={focusedProject ? "Find a task…" : "Find a project…"} /></label></div>
        {!focusedProject ? <nav className="observatory-regions" aria-label="Portfolio regions"><button type="button" aria-pressed={region === null} onClick={() => chooseRegion(null)}>All regions</button>{portfolioRegions.map((item) => <button type="button" key={item.id} aria-pressed={region === item.id} onClick={() => chooseRegion(item.id)}><span aria-hidden="true" style={{ color: item.color }}>{item.badge}</span><strong>{item.title}</strong><span>{projects.filter((project) => projectRegion(directory[project.id]?.overview, now) === item.id).length} projects</span><small>{item.explanation}</small></button>)}</nav> : null}
        {!focusedProject && (region || normalizedQuery) && matchingProjects.length > PORTFOLIO_PAGE_SIZE ? <div className="observatory-pagination" aria-label="Project pages"><button type="button" disabled={displayedPage === 0} onClick={() => setRegionPage(displayedPage - 1)}>Previous projects</button><span>Page {displayedPage + 1} of {pageCount}</span><button type="button" disabled={displayedPage + 1 >= pageCount} onClick={() => setRegionPage(displayedPage + 1)}>Next projects</button></div> : null}
        <div className="observatory-directory__items" role="list">
          {focusedProject ? visibleTasks.map((task: Task) => <div role="listitem" key={task.id}><button className={`observatory-card ${selected?.id === task.id ? "is-selected" : ""}`} type="button" onClick={() => selectTask(task.id)}><span className={`observatory-card__mark observatory-card__mark--${task.status}`} aria-hidden="true" /><strong>{task.title}</strong><span>{task.status} · {task.dependencies.length} dependencies</span></button></div>) : visibleProjects.map((project) => <div role="listitem" key={project.id}><button className="observatory-card" type="button" onClick={() => selectProject(project.id)}><span className="observatory-card__mark" aria-hidden="true" /><strong>{project.name}</strong><span>{directory[project.id]?.overview ? `${directory[project.id]?.overview?.counts.task ?? 0} tasks · ${directory[project.id]?.overview?.counts.attention ?? 0} attention records` : directory[project.id]?.state === "denied" ? "Summary access restricted" : directory[project.id]?.state === "error" ? "Summary unavailable" : "Reading summary…"}</span>{directory[project.id]?.overview?.sources?.map((source) => <SourceFreshness key={source.system} source={source} compact />)}</button></div>)}
        </div>
        {(focusedProject ? visibleTasks.length : visibleProjects.length) === 0 ? <p className="observatory-muted">{query ? "Nothing matches your search." : focusedProject ? "No tasks have been recorded for this project." : !projects.length ? "No projects are available to your account." : !region && projects.length > PORTFOLIO_PAGE_SIZE ? "Choose a region or search to unfold the project directory." : "No projects are available in this region."}</p> : null}
      </section>


    </div>
  );
}

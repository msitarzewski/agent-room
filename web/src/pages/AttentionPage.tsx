import { useCallback, useEffect, useState } from "react";
import { api, ApiError } from "../api/client";
import type { AttentionItem, ProjectBrief } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import {
  ActionFeedback,
  Freshness,
  PageHeader,
  RelativeTime,
  StatePanel,
  StatusPill,
  hasCapability,
} from "../components/Common";
import { useCollection } from "../hooks/useCollection";
import type { LoadState } from "../hooks/useCollection";

export function AttentionPage() {
  const { projectId, projects } = useAuth();
  const collection = useCollection(api.attention);
  const [brief, setBrief] = useState<ProjectBrief | null>(null);
  const [briefState, setBriefState] = useState<LoadState>("loading");
  const [briefError, setBriefError] = useState<string | null>(null);
  const [reviewedThrough, setReviewedThrough] = useState<number | null>(null);
  const [briefBusy, setBriefBusy] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const loadBrief = useCallback(async (signal?: AbortSignal) => {
    if (!projectId) {
      setBrief(null);
      setBriefState("empty");
      return;
    }
    setBriefError(null);
    try {
      const next = await api.brief(projectId, 0, signal);
      setBrief(next);
      setBriefState("ready");
      setReviewedThrough(null);
    } catch (caught) {
      if (signal?.aborted) return;
      if (caught instanceof ApiError && (caught.status === 401 || caught.status === 403)) {
        setBriefState("denied");
      } else {
        setBriefState("error");
        setBriefError(caught instanceof Error ? caught.message : "Unable to load the return brief.");
      }
    }
  }, [projectId]);

  useEffect(() => {
    const controller = new AbortController();
    setBriefState("loading");
    void loadBrief(controller.signal);
    const onStream = () => void loadBrief();
    window.addEventListener("agentroom:stream", onStream);
    return () => {
      controller.abort();
      window.removeEventListener("agentroom:stream", onStream);
    };
  }, [loadBrief]);

  const act = async (item: AttentionItem, action: "acknowledge" | "resolve") => {
    setBusyId(item.id);
    setMessage(null);
    setActionError(null);
    try {
      await api.attentionAction(item, action);
      setMessage(`${item.title} was ${action === "acknowledge" ? "acknowledged" : "resolved"}.`);
      await collection.reload();
    } catch (caught) {
      setActionError(caught instanceof Error ? caught.message : "The attention item could not be updated.");
    } finally {
      setBusyId(null);
    }
  };

  const open = collection.items.filter((item) => item.status !== "resolved");
  const critical = open.filter((item) => item.severity === "critical").length;
  const canManage = hasCapability(
    projects.find((project) => project.id === projectId)?.capabilities ?? [],
    "attention:manage",
  );
  const awaiting = canManage ? open.length : 0;
  const reviewBrief = async () => {
    if (!brief || !projectId) return;
    setBriefBusy(true);
    setActionError(null);
    try {
      const result = await api.acknowledgeBrief(projectId, brief.reviewed_cursor, brief.through_cursor);
      setReviewedThrough(result.reviewed_cursor);
      setMessage(`Changes through event ${result.reviewed_cursor} were marked reviewed.`);
    } catch (caught) {
      setActionError(caught instanceof Error ? caught.message : "The return brief could not be acknowledged.");
    } finally {
      setBriefBusy(false);
    }
  };

  return (
    <div className="page">
      <PageHeader
        eyebrow="Command brief"
        title="What needs you now"
        description="Decisions and exceptions distilled from worker activity—ordered by urgency, evidence, and ownership."
        actions={<button className="button button--secondary" type="button" onClick={() => void collection.reload()}>Refresh</button>}
      />
      <Freshness loadedAt={collection.lastLoadedAt} />
      <ActionFeedback message={message} error={actionError} />
      <section aria-labelledby="return-brief-heading">
        <div className="section-heading">
          <h2 id="return-brief-heading">Since you were away</h2>
          {brief ? <span>Events {brief.from_cursor}–{brief.through_cursor}</span> : null}
        </div>
        <StatePanel
          state={briefState}
          error={briefError}
          emptyTitle="No return brief available"
          emptyBody="Select a project to load its durable activity brief."
          onRetry={() => void loadBrief()}
        >
          {brief ? (
            <article className="detail-card">
              <div className="card-heading">
                <div>
                  <p className="eyebrow">Deterministic project summary</p>
                  <h3>{brief.events.length} changes · {brief.open_attention.length} open · {brief.pending_approvals.length} approvals</h3>
                </div>
                <Freshness loadedAt={brief.generated_at} />
              </div>
              {brief.recommended_actions.length > 0 ? (
                <div>
                  <strong>Recommended next actions</strong>
                  <ul>{brief.recommended_actions.map((action) => <li key={action}>{action}</li>)}</ul>
                </div>
              ) : <p>No new action is recommended.</p>}
              <dl className="metadata-row">
                {Object.entries(brief.event_counts).map(([eventType, count]) => (
                  <div key={eventType}><dt>{eventType}</dt><dd>{count}</dd></div>
                ))}
              </dl>
              {brief.through_cursor > (reviewedThrough ?? brief.reviewed_cursor) ? (
                <button className="button button--secondary" disabled={briefBusy} type="button" onClick={() => void reviewBrief()}>Mark changes reviewed</button>
              ) : <span className="muted">This brief is reviewed through event {brief.through_cursor}.</span>}
            </article>
          ) : null}
        </StatePanel>
      </section>
      {collection.state === "ready" ? (
        <section className="metric-grid" aria-label="Attention summary">
          <div className="metric"><span>Open</span><strong>{open.length}</strong><small>attention items</small></div>
          <div className="metric metric--critical"><span>Critical</span><strong>{critical}</strong><small>immediate review</small></div>
          <div className="metric"><span>Actionable</span><strong>{awaiting}</strong><small>you can act now</small></div>
        </section>
      ) : null}
      <StatePanel
        state={collection.state}
        error={collection.error}
        emptyTitle="The room is quiet"
        emptyBody="No decisions or exceptions need your attention. New situations will appear here automatically."
        onRetry={() => void collection.reload()}
      >
        <section aria-labelledby="attention-list-heading">
          <div className="section-heading">
            <h2 id="attention-list-heading">Attention inbox</h2>
            <span>{open.length} open</span>
          </div>
          <div className="attention-list">
            {collection.items.map((item) => (
              <article className={`attention-card attention-card--${item.severity}`} key={item.id}>
                <div className="attention-accent" aria-hidden="true" />
                <div className="attention-body">
                  <div className="card-heading">
                    <div><StatusPill value={item.severity} /><h3>{item.title}</h3></div>
                    <StatusPill value={item.status} />
                  </div>
                  <p>{item.detail ?? "No additional detail was supplied."}</p>
                  <dl className="metadata-row">
                    <div><dt>Kind</dt><dd>{item.kind}</dd></div>
                    <div><dt>Owner</dt><dd>{item.owner_id ?? "Unassigned"}</dd></div>
                    <div><dt>Updated</dt><dd><RelativeTime value={item.updated_at} /></dd></div>
                    <div><dt>Resource</dt><dd>{item.resource_type && item.resource_id ? `${item.resource_type} · ${item.resource_id}` : "Not linked"}</dd></div>
                  </dl>
                  {item.status !== "resolved" ? (
                    <div className="card-actions">
                      {canManage && item.status === "open" ? (
                        <button
                          className="button button--secondary"
                          type="button"
                          disabled={busyId === item.id}
                          onClick={() => void act(item, "acknowledge")}
                        >
                          Acknowledge
                        </button>
                      ) : null}
                      {canManage ? (
                        <button
                          className="button button--primary"
                          type="button"
                          disabled={busyId === item.id}
                          onClick={() => void act(item, "resolve")}
                        >
                          Mark resolved
                        </button>
                      ) : null}
                      {!canManage ? (
                        <span className="muted">No actions are authorized for your role.</span>
                      ) : null}
                    </div>
                  ) : null}
                </div>
              </article>
            ))}
          </div>
        </section>
      </StatePanel>
    </div>
  );
}

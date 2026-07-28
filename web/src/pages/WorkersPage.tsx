import { useState } from "react";
import { api } from "../api/client";
import type { Approval, Run } from "../api/types";
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

function approvedCancellation(approvals: Approval[], run: Run): Approval | undefined {
  return approvals.find((approval) =>
    approval.status === "approved" &&
    !approval.consumed_at &&
    Date.parse(approval.expires_at) > Date.now() &&
    approval.command_version === 1 &&
    approval.expected_target_version === run.version &&
    approval.command_digest.startsWith("sha256:") &&
    approval.resource_type === "run" &&
    approval.resource_id === run.id &&
    approval.context?.action === "cancel",
  );
}

function RunControls({ run, actorCapabilities, approvals, onChanged }: {
  run: Run;
  actorCapabilities: string[];
  approvals: Approval[];
  onChanged: () => Promise<void>;
}) {
  const [mode, setMode] = useState<"message" | "redirect" | "cancel" | null>(null);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const runTitle = run.summary || `Run ${run.id}`;

  const execute = async (action: string, value?: string, approvalId?: string) => {
    setBusy(true);
    setMessage(null);
    setError(null);
    try {
      await api.runAction(run, action, value, approvalId);
      setMessage(`${action} command accepted for ${runTitle}.`);
      setMode(null);
      setText("");
      await onChanged();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "The run action failed.");
    } finally {
      setBusy(false);
    }
  };

  const requestCancellation = async () => {
    setBusy(true);
    setMessage(null);
    setError(null);
    try {
      await api.requestRunApproval(run, "cancel");
      setMessage(`Cancellation approval requested for ${runTitle}.`);
      await onChanged();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "The approval request failed.");
    } finally {
      setBusy(false);
    }
  };

  const simpleActions =
    run.status === "paused"
      ? [{ key: "resume", label: "Resume" }]
      : run.status === "working" || run.status === "running"
        ? [{ key: "pause", label: "Pause" }]
        : [];
  const canAct = (action: string) =>
    hasCapability(actorCapabilities, `run:${action}`) &&
    hasCapability(run.capabilities, `run:${action}`);
  const cancellationApproval = approvedCancellation(approvals, run);

  return (
    <div className="run-controls">
      <ActionFeedback message={message} error={error} />
      <div className="card-actions">
        {simpleActions.map((action) =>
          canAct(action.key) ? (
            <button
              className="button button--secondary"
              type="button"
              disabled={busy}
              key={action.key}
              onClick={() => void execute(action.key)}
            >
              {action.label}
            </button>
          ) : null,
        )}
        {canAct("message") ? (
          <button className="button button--secondary" type="button" onClick={() => setMode("message")}>Message</button>
        ) : null}
        {canAct("redirect") ? (
          <button className="button button--secondary" type="button" onClick={() => setMode("redirect")}>Redirect</button>
        ) : null}
        {canAct("cancel") && cancellationApproval ? (
          <button className="button button--danger" type="button" onClick={() => setMode("cancel")}>Cancel run</button>
        ) : null}
        {canAct("cancel") && !cancellationApproval && hasCapability(actorCapabilities, "approval:request") ? (
          <button className="button button--danger" disabled={busy} type="button" onClick={() => void requestCancellation()}>Request cancellation approval</button>
        ) : null}
        {canAct("cancel") && !cancellationApproval ? (
          <span className="muted">Cancellation remains unavailable until an exact, unexpired request is approved in <a href="/approvals">Approvals</a>.</span>
        ) : null}
        {run.capabilities.filter((capability) => capability.startsWith("run:")).length === 0 ? (
          <span className="muted">This adapter exposes observation only.</span>
        ) : null}
      </div>
      {mode === "message" || mode === "redirect" ? (
        <form
          className="inline-form"
          onSubmit={(event) => {
            event.preventDefault();
            void execute(mode, text.trim());
          }}
        >
          <label>
            <span>{mode === "message" ? "Message for the next safe turn" : "New direction for this run"}</span>
            <textarea required value={text} onChange={(event) => setText(event.target.value)} />
          </label>
          <div>
            <button className="button button--primary" disabled={busy || !text.trim()} type="submit">Send {mode}</button>
            <button className="button button--secondary" type="button" onClick={() => setMode(null)}>Keep running</button>
          </div>
        </form>
      ) : null}
      {mode === "cancel" ? (
        <div className="confirm-panel" role="alert">
          <strong>Cancel {runTitle}?</strong>
          <p>Agent Room will request graceful termination. The audit trail will retain this action.</p>
          <div>
            <button className="button button--danger" disabled={busy || !cancellationApproval} type="button" onClick={() => void execute("cancel", undefined, cancellationApproval?.id)}>Submit approved cancellation</button>
            <button className="button button--secondary" type="button" onClick={() => setMode(null)}>Keep running</button>
          </div>
        </div>
      ) : null}
    </div>
  );
}

export function WorkersPage() {
  const { projectId, projects } = useAuth();
  const agents = useCollection(api.agents);
  const runs = useCollection(api.runs);
  const approvals = useCollection(api.approvals);
  const [filter, setFilter] = useState("all");
  const visibleRuns = runs.items.filter((run) => filter === "all" || run.status === filter);
  const actorCapabilities = projects.find((project) => project.id === projectId)?.capabilities ?? [];

  return (
    <div className="page">
      <PageHeader
        eyebrow="Fleet awareness"
        title="Workers & runs"
        description="Persistent identities and their current executions, normalized across runtimes."
        actions={
          <label className="compact-field"><span>Run state</span>
            <select value={filter} onChange={(event) => setFilter(event.target.value)}>
              <option value="all">All states</option><option value="running">Running</option><option value="working">Working</option>
              <option value="paused">Paused</option><option value="blocked">Blocked</option>
              <option value="failed">Failed</option><option value="disconnected">Disconnected</option>
            </select>
          </label>
        }
      />
      <Freshness loadedAt={runs.lastLoadedAt} />
      <section aria-labelledby="worker-heading">
        <div className="section-heading"><h2 id="worker-heading">Persistent workers</h2><span>{agents.items.length} registered</span></div>
        <StatePanel
          state={agents.state}
          error={agents.error}
          emptyTitle="No workers connected"
          emptyBody="Connect a supported runtime adapter to register the first worker."
          onRetry={() => void agents.reload()}
        >
          <div className="worker-grid">
            {agents.items.map((agent) => (
              <article className="worker-card" key={agent.id}>
                <div className="worker-avatar">{agent.display_name.slice(0, 2).toUpperCase()}</div>
                <div>
                  <div className="card-heading"><h3>{agent.display_name}</h3><StatusPill value={agent.status} /></div>
                  <p>{agent.kind} · {agent.source.system}</p>
                  <small>Updated <RelativeTime value={agent.updated_at} />{agent.source.external_id ? ` · external ${agent.source.external_id}` : ""}</small>
                </div>
              </article>
            ))}
          </div>
        </StatePanel>
      </section>
      <section aria-labelledby="runs-heading">
        <div className="section-heading"><h2 id="runs-heading">Runs</h2><span>{visibleRuns.length} shown</span></div>
        <StatePanel
          state={runs.state}
          error={runs.error}
          emptyTitle="No runs recorded"
          emptyBody="Real executions will appear when a connected worker begins work."
          onRetry={() => void runs.reload()}
        >
          {visibleRuns.length === 0 ? <div className="state-panel"><strong>No runs match this filter.</strong></div> : (
            <div className="card-list">
              {visibleRuns.map((run) => (
                <article className="detail-card" key={run.id}>
                  <div className="card-heading">
                    <div><p className="eyebrow">{run.agent_id} · {run.source.system}</p><h3>{run.summary || `Run ${run.id}`}</h3></div>
                    <StatusPill value={run.status} />
                  </div>
                  <dl className="metadata-row">
                    <div><dt>Created</dt><dd><RelativeTime value={run.created_at} /></dd></div>
                    <div><dt>Updated</dt><dd><RelativeTime value={run.updated_at} /></dd></div>
                    <div><dt>External ID</dt><dd><code>{run.source.external_id ?? "Not supplied"}</code></dd></div>
                  </dl>
                  <RunControls run={run} actorCapabilities={actorCapabilities} approvals={approvals.items} onChanged={async () => {
                    await runs.reload();
                    await approvals.reload();
                  }} />
                </article>
              ))}
            </div>
          )}
        </StatePanel>
      </section>
    </div>
  );
}

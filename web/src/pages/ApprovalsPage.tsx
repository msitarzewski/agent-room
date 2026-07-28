import { useState } from "react";
import { api } from "../api/client";
import type { Approval } from "../api/types";
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

function approvalAction(approval: Approval): string {
  const action = approval.context?.action;
  return typeof action === "string" && action ? action : approval.kind;
}

export function ApprovalsPage() {
  const { projectId, projects } = useAuth();
  const collection = useCollection(api.approvals);
  const [decision, setDecision] = useState<{ approval: Approval; value: "approved" | "rejected" } | null>(null);
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const canDecide = hasCapability(
    projects.find((project) => project.id === projectId)?.capabilities ?? [],
    "approval:decide",
  );

  const submit = async () => {
    if (!decision) return;
    setBusy(true);
    setMessage(null);
    setError(null);
    try {
      await api.decideApproval(decision.approval, decision.value, note.trim() || undefined);
      setMessage(`${approvalAction(decision.approval)} was ${decision.value}.`);
      setDecision(null);
      setNote("");
      await collection.reload();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "The approval decision failed.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="page">
      <PageHeader
        eyebrow="Human authority"
        title="Approvals"
        description="Inspect the exact action, target, expiry, and command digest before authorizing destructive work."
      />
      <Freshness loadedAt={collection.lastLoadedAt} />
      <ActionFeedback message={message} error={error} />
      <StatePanel
        state={collection.state}
        error={collection.error}
        emptyTitle="No approval requests"
        emptyBody="Destructive controls remain unavailable until a bound, time-limited approval request exists."
        onRetry={() => void collection.reload()}
      >
        <div className="card-list">
          {collection.items.map((approval) => (
            <article className="detail-card" key={approval.id}>
              <div className="card-heading">
                <div><p className="eyebrow">{approval.resource_type}</p><h2>{approvalAction(approval)} · {approval.resource_id}</h2></div>
                <StatusPill value={approval.status} />
              </div>
              <p>{approval.reason}</p>
              <dl className="metadata-grid">
                <div><dt>Requested by</dt><dd><code>{approval.requested_by}</code></dd></div>
                <div><dt>Expires</dt><dd><RelativeTime value={approval.expires_at} /></dd></div>
                <div><dt>Command version</dt><dd>{approval.command_version}</dd></div>
                <div><dt>Target version</dt><dd>{approval.expected_target_version}</dd></div>
                <div><dt>Consumed</dt><dd><RelativeTime value={approval.consumed_at ?? null} /></dd></div>
                <div className="metadata-wide"><dt>Exact command digest</dt><dd><code>{approval.command_digest}</code></dd></div>
              </dl>
              {approval.status === "pending" && canDecide ? (
                <div className="card-actions">
                  <button className="button button--primary" type="button" onClick={() => setDecision({ approval, value: "approved" })}>Review to approve</button>
                  <button className="button button--danger" type="button" onClick={() => setDecision({ approval, value: "rejected" })}>Review to reject</button>
                </div>
              ) : approval.status === "pending" ? <span className="muted">Your role cannot decide approvals.</span> : null}
            </article>
          ))}
        </div>
      </StatePanel>
      {decision ? (
        <div className="confirm-panel" role="alertdialog" aria-labelledby="approval-confirm-title">
          <strong id="approval-confirm-title">{decision.value === "approved" ? "Approve" : "Reject"} exact action?</strong>
          <p><code>{approvalAction(decision.approval)}</code> on <code>{decision.approval.resource_type}:{decision.approval.resource_id}</code></p>
          <label>
            <span>Decision note</span>
            <textarea value={note} onChange={(event) => setNote(event.target.value)} maxLength={2000} />
          </label>
          <div>
            <button className={decision.value === "approved" ? "button button--primary" : "button button--danger"} disabled={busy} type="button" onClick={() => void submit()}>
              Confirm {decision.value === "approved" ? "approval" : "rejection"}
            </button>
            <button className="button button--secondary" disabled={busy} type="button" onClick={() => setDecision(null)}>Keep pending</button>
          </div>
        </div>
      ) : null}
    </div>
  );
}

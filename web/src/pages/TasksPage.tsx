import { useState } from "react";
import { api } from "../api/client";
import type { Task, TaskStatus } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import {
  ActionFeedback,
  Freshness,
  PageHeader,
  StatePanel,
  StatusPill,
  hasCapability,
} from "../components/Common";
import { useCollection } from "../hooks/useCollection";

const transitions: Partial<Record<TaskStatus, TaskStatus[]>> = {
  inbox: ["ready", "cancelled", "failed"],
  ready: ["working", "blocked", "cancelled", "failed"],
  working: ["review", "blocked", "cancelled", "failed"],
  review: ["working", "completed", "blocked", "cancelled", "failed"],
  completed: ["archived", "reopened"],
  reopened: ["ready", "cancelled", "failed"],
  blocked: ["ready", "working", "cancelled", "failed"],
  failed: ["reopened", "cancelled"],
};

export function TasksPage() {
  const { projectId, projects } = useAuth();
  const collection = useCollection(api.tasks);
  const claims = useCollection(api.claims);
  const [statusFilter, setStatusFilter] = useState("active");
  const [busyId, setBusyId] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [blockPrompt, setBlockPrompt] = useState<Task | null>(null);
  const [blockReason, setBlockReason] = useState("");
  const visible = collection.items.filter((task) =>
    statusFilter === "all"
      ? true
      : statusFilter === "active"
        ? !["archived", "cancelled", "completed"].includes(task.status)
        : task.status === statusFilter,
  );
  const projectCapabilities = projects.find((project) => project.id === projectId)?.capabilities ?? [];
  const canTransition = hasCapability(projectCapabilities, "task:transition");
  const canReview = hasCapability(projectCapabilities, "claim:review");

  const execute = async (task: Task, operation: () => Promise<unknown>, description: string) => {
    setBusyId(task.id);
    setMessage(null);
    setError(null);
    try {
      await operation();
      setMessage(description);
      await collection.reload();
      await claims.reload();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "The task could not be updated.");
    } finally {
      setBusyId(null);
    }
  };

  return (
    <div className="page">
      <PageHeader
        eyebrow="Work state"
        title="Tasks & review"
        description="Explicit task transitions, evidence requirements, and review decisions—never inferred from chat."
        actions={
          <label className="compact-field"><span>Status</span>
            <select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
              <option value="active">Active work</option><option value="all">All tasks</option>
              <option value="review">Awaiting review</option><option value="blocked">Blocked</option>
              <option value="completed">Completed</option><option value="failed">Failed</option>
            </select>
          </label>
        }
      />
      <Freshness loadedAt={collection.lastLoadedAt} />
      <ActionFeedback message={message} error={error} />
      <StatePanel
        state={collection.state}
        error={collection.error}
        emptyTitle="No tasks yet"
        emptyBody="Tasks created by authorized people or connected workers will appear here."
        onRetry={() => void collection.reload()}
      >
        {visible.length === 0 ? <div className="state-panel"><strong>No tasks match this filter.</strong></div> : (
          <div className="task-board">
            {visible.map((task) => (
              <article className={`task-card task-card--${task.priority}`} key={task.id}>
                {(() => {
                  const claim = claims.items.find((item) => item.task_id === task.id && !["accepted", "rejected"].includes(item.status));
                  return (
                    <>
                <div className="card-heading">
                  <div><p className="eyebrow">{task.source.system} · {task.priority} priority</p><h2>{task.title}</h2></div>
                  <StatusPill value={task.status} />
                </div>
                {task.description ? <p>{task.description}</p> : null}
                {task.blocked_reason ? <div className="inline-notice inline-notice--danger"><strong>Blocked:</strong> {task.blocked_reason}</div> : null}
                <dl className="metadata-row">
                  <div><dt>Owner</dt><dd>{task.owner_id ?? "Unassigned"}</dd></div>
                  <div><dt>Review</dt><dd><StatusPill value={task.review_state ?? "not requested"} /></dd></div>
                  <div><dt>Approval</dt><dd><StatusPill value={task.approval_state ?? "not required"} /></dd></div>
                  <div><dt>Dependencies</dt><dd>{task.dependencies.length}</dd></div>
                </dl>
                <div className="card-actions" aria-label={`Actions for ${task.title}`}>
                  {(transitions[task.status] ?? []).map((next) =>
                    canTransition ? (
                      <button
                        className={next === "cancelled" || next === "failed" ? "button button--danger" : "button button--secondary"}
                        disabled={busyId === task.id}
                        type="button"
                        key={next}
                        onClick={() => {
                          if (next === "blocked") {
                            setBlockPrompt(task);
                            setBlockReason("");
                          } else {
                            void execute(task, () => api.transitionTask(task, next), `${task.title} moved to ${next}.`);
                          }
                        }}
                      >
                        Move to {next}
                      </button>
                    ) : null,
                  )}
                  {task.status === "review" && claim && canReview ? (
                    <button
                      className="button button--primary"
                      disabled={busyId === task.id}
                      type="button"
                      onClick={() => void execute(task, () => api.reviewClaim(claim, "accepted"), `${task.title} was approved.`)}
                    >Approve evidence</button>
                  ) : null}
                  {task.status === "review" && claim && canReview ? (
                    <button
                      className="button button--danger"
                      disabled={busyId === task.id}
                      type="button"
                      onClick={() => void execute(task, () => api.reviewClaim(claim, "rejected"), `${task.title} was rejected.`)}
                    >Reject claim</button>
                  ) : null}
                  {!canTransition && !canReview ? <span className="muted">Read-only for your role.</span> : null}
                </div>
                    </>
                  );
                })()}
              </article>
            ))}
          </div>
        )}
      </StatePanel>
      {blockPrompt ? (
        <form
          className="confirm-panel"
          role="alertdialog"
          aria-labelledby="block-task-title"
          onSubmit={(event) => {
            event.preventDefault();
            const reason = blockReason.trim();
            if (!reason) return;
            const taskToBlock = blockPrompt;
            setBlockPrompt(null);
            void execute(taskToBlock, () => api.transitionTask(taskToBlock, "blocked", reason), `${taskToBlock.title} was blocked.`);
          }}
        >
          <strong id="block-task-title">Why is {blockPrompt.title} blocked?</strong>
          <p>This reason becomes part of the durable task state and audit trail.</p>
          <label>
            <span>Blocking reason</span>
            <textarea required maxLength={2000} value={blockReason} onChange={(event) => setBlockReason(event.target.value)} />
          </label>
          <div>
            <button className="button button--danger" disabled={!blockReason.trim()} type="submit">Confirm blocked state</button>
            <button className="button button--secondary" type="button" onClick={() => setBlockPrompt(null)}>Keep current state</button>
          </div>
        </form>
      ) : null}
    </div>
  );
}

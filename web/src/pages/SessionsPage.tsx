import { api } from "../api/client";
import { Freshness, PageHeader, RelativeTime, StatePanel, StatusPill } from "../components/Common";
import { useCollection } from "../hooks/useCollection";

export function SessionsPage() {
  const collection = useCollection(api.sessions);
  return (
    <div className="page">
      <PageHeader
        eyebrow="Conversational context"
        title="Active sessions"
        description="Runtime-owned sessions are linked by native identity; Agent Room does not duplicate provider memory or conversation authority."
      />
      <Freshness loadedAt={collection.lastLoadedAt} />
      <StatePanel
        state={collection.state}
        error={collection.error}
        emptyTitle="No sessions observed"
        emptyBody="Connected runtime sessions will appear after their first supported lifecycle signal."
        onRetry={() => void collection.reload()}
      >
        <div className="card-list">
          {collection.items.map((session) => (
            <article className="detail-card" key={session.id}>
              <div className="card-heading"><div><p className="eyebrow">{session.source.system}</p><h2>{session.conversation_ref || session.external_ref || `Session ${session.id}`}</h2></div><StatusPill value={session.status} /></div>
              <dl className="metadata-row">
                <div><dt>Agent</dt><dd>{session.agent_id}</dd></div>
                <div><dt>Created</dt><dd><RelativeTime value={session.created_at} /></dd></div>
                <div><dt>Updated</dt><dd><RelativeTime value={session.updated_at} /></dd></div>
                <div><dt>Reconciled</dt><dd><RelativeTime value={session.last_reconciled_at ?? null} /></dd></div>
                <div><dt>Native ID</dt><dd><code>{session.external_ref ?? session.source.external_id ?? "Not supplied"}</code></dd></div>
              </dl>
            </article>
          ))}
        </div>
      </StatePanel>
    </div>
  );
}

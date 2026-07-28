import { useMemo, useState } from "react";
import { api } from "../api/client";
import { Freshness, PageHeader, RelativeTime, StatePanel, StatusPill } from "../components/Common";
import { useCollection } from "../hooks/useCollection";

const loadEvents = (projectId: string, signal?: AbortSignal) =>
  api.events(projectId, undefined, signal);

export function TimelinePage() {
  const collection = useCollection(loadEvents);
  const [query, setQuery] = useState("");
  const visible = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return collection.items.filter(
      (event) =>
        (!normalized ||
          event.type.toLowerCase().includes(normalized) ||
          event.subject_type.toLowerCase().includes(normalized) ||
          event.subject_id.toLowerCase().includes(normalized) ||
          event.actor_id.toLowerCase().includes(normalized)),
    );
  }, [collection.items, query]);

  return (
    <div className="page">
      <PageHeader
        eyebrow="Semantic history"
        title="Timeline"
        description="Durable coordination facts with complete source provenance. Raw model telemetry remains outside this view."
      />
      <div className="filter-bar" role="search">
        <label><span>Filter events</span><input type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Type, subject, or actor" /></label>
      </div>
      <Freshness loadedAt={collection.lastLoadedAt} />
      <StatePanel
        state={collection.state}
        error={collection.error}
        emptyTitle="No semantic events"
        emptyBody="Meaningful worker and task milestones will appear after an adapter reports real work."
        onRetry={() => void collection.reload()}
      >
        {visible.length === 0 ? <div className="state-panel"><strong>No events match your filters.</strong></div> : (
          <ol className="timeline-list" aria-label="Event timeline">
            {visible.map((event) => (
              <li key={event.id}>
                <span className="timeline-node timeline-node--normal" aria-hidden="true" />
                <article>
                  <div className="card-heading">
                    <div><p className="eyebrow">{event.type}</p><h2>{event.subject_type} · {event.subject_id}</h2></div>
                    <StatusPill value={`schema v${event.schema_version}`} />
                  </div>
                  <dl className="metadata-row">
                    <div><dt>Occurred</dt><dd><RelativeTime value={event.occurred_at} /></dd></div>
                    <div><dt>Actor</dt><dd><code>{event.actor_id}</code></dd></div>
                    <div><dt>Correlation</dt><dd><code>{event.correlation_id ?? "Not supplied"}</code></dd></div>
                    <div><dt>Cursor</dt><dd>{event.cursor}</dd></div>
                  </dl>
                </article>
              </li>
            ))}
          </ol>
        )}
      </StatePanel>
    </div>
  );
}

import { useCallback, useEffect, useRef, useState } from "react";
import { api, ApiError } from "../api/client";
import type { EventPage, PageCursor, SemanticEvent, StreamMessage } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { Freshness, PageHeader, RelativeTime, StatePanel, StatusPill } from "../components/Common";
import { useProjectResource } from "../hooks/useCollection";
import { subscribeProjectChanges, useStream } from "../stream/StreamContext";

const PAGE_SIZE = 50;
const emptyPage = (page: EventPage) => page.items.length === 0;

function streamEvent(message: StreamMessage, projectId: string): SemanticEvent | undefined {
  const event = message.data as Partial<SemanticEvent> | null;
  if (event?.project_id === projectId && event.type === message.type && event.cursor === message.cursor &&
    Number.isSafeInteger(event.cursor) && typeof event.id === "string" && typeof event.actor_id === "string" &&
    typeof event.subject_type === "string" && typeof event.subject_id === "string" &&
    typeof event.occurred_at === "string" && Number.isFinite(Date.parse(event.occurred_at)) &&
    typeof event.schema_version === "number") return event as SemanticEvent;
}

function newestFirst(events: SemanticEvent[]) {
  const byCursor = new Map(events.map((event) => [event.cursor, event]));
  // Date.parse resolves timezone offsets but drops sub-millisecond precision.
  // Compare whole seconds and PostgreSQL's six fractional digits separately.
  const ordered = Array.from(byCursor.values(), (event) => {
    const fraction = event.occurred_at.match(/\.(\d+)/)?.[1] ?? "";
    return {
      event,
      second: Date.parse(event.occurred_at.replace(/\.\d+/, "")),
      microsecond: Number(fraction.slice(0, 6).padEnd(6, "0")),
    };
  });
  ordered.sort((a, b) => b.second - a.second || b.microsecond - a.microsecond || b.event.cursor - a.event.cursor);
  return ordered.map(({ event }) => event);
}

export function TimelinePage() {
  const { projectId } = useAuth();
  return <ProjectTimeline key={projectId} projectId={projectId} />;
}

function ProjectTimeline({ projectId }: { projectId: string | null }) {
  const [input, setInput] = useState("");
  const [query, setQuery] = useState("");
  const [eventType, setEventType] = useState("");
  const [types, setTypes] = useState<string[]>([]);
  const catalog = useRef({ complete: false, denied: false });
  const addTypes = useCallback((incoming: string[]) => {
    if (catalog.current.denied) return;
    setTypes((current) => {
      if (incoming.every((type) => current.includes(type))) return current;
      return [...new Set([...current, ...incoming])].sort();
    });
  }, []);
  const clearTypes = useCallback((denied: boolean) => {
    catalog.current = { complete: false, denied };
    setTypes([]);
  }, []);
  const readEvent = useCallback((message: StreamMessage, id: string) => {
    if (catalog.current.denied) return;
    const event = streamEvent(message, id);
    if (event) addTypes([event.type]);
    return event;
  }, [addTypes]);
  const loadPage = useCallback(async (id: string, before: PageCursor | undefined, signal: AbortSignal) => {
    const current = catalog.current;
    const page = await api.timeline(id, { limit: PAGE_SIZE, before, query, eventType, includeTypes: !current.complete }, signal);
    if (!signal.aborted && catalog.current === current) {
      if (page.event_types) {
        current.complete = true;
        current.denied = false;
        if (eventType && !page.event_types.includes(eventType)) setEventType("");
      }
      addTypes(page.event_types ?? page.items.map((event) => event.type));
    }
    return { ...page, before };
  }, [addTypes, eventType, query]);
  useEffect(() => {
    const next = input.trim();
    if (next === query) return;
    const timer = window.setTimeout(() => setQuery(next), 250);
    return () => window.clearTimeout(timer);
  }, [input, query]);

  return (
    <div className="page">
      <PageHeader
        eyebrow="Semantic history"
        title="Timeline"
        description="Durable coordination facts with complete source provenance. Raw model telemetry remains outside this view."
      />
      <div className="filter-bar" role="search">
        <label><span>Filter events</span><input type="search" value={input} onChange={(event) => setInput(event.target.value)} placeholder="Type, subject, or actor" /></label>
        <label><span>Event type</span><select value={eventType} onChange={(event) => setEventType(event.target.value)}>
          <option value="">All event types</option>
          {types.map((type) => <option key={type} value={type}>{type}</option>)}
        </select></label>
      </div>
      <TimelineHistory key={JSON.stringify([query, eventType])} projectId={projectId} query={query} eventType={eventType}
        loadPage={loadPage} readEvent={readEvent} clearTypes={clearTypes} />
    </div>
  );
}

function TimelineHistory({ projectId, query, eventType, loadPage, readEvent, clearTypes }: {
  projectId: string | null;
  query: string;
  eventType: string;
  loadPage: (id: string, before: PageCursor | undefined, signal: AbortSignal) => Promise<EventPage & { before?: PageCursor }>;
  readEvent: (message: StreamMessage, id: string) => SemanticEvent | undefined;
  clearTypes: (denied: boolean) => void;
}) {
  const { status } = useStream();
  // A new connection may immediately request a snapshot after its replay limit.
  const [initialDelay] = useState(status === "connected" ? 0 : 250);
  const [cursors, setCursors] = useState<PageCursor[]>([]);
  const history = useRef<HTMLDivElement>(null);
  const before = cursors.at(-1);
  const [live, setLive] = useState<{ events: SemanticEvent[]; receivedAt: string } | null>(null);
  const clearSnapshot = useCallback((denied: boolean) => {
    setLive(null);
    clearTypes(denied);
  }, [clearTypes]);
  const load = useCallback(async (id: string, signal: AbortSignal) => {
    try {
      return await loadPage(id, before, signal);
    } catch (error) {
      if (!signal.aborted && error instanceof ApiError && [401, 403].includes(error.status)) clearSnapshot(true);
      throw error;
    }
  }, [before, clearSnapshot, loadPage]);
  const subscribe = useCallback((id: string, refresh: () => void, deny: () => void) =>
    subscribeProjectChanges(id, refresh, () => { clearSnapshot(true); deny(); }, (message) => {
      if (message.type === "stream_connected" && !(message.data as { reconnected?: boolean })?.reconnected) return false;
      if (message.type === "stream_connected" || message.type === "resync_required") {
        clearSnapshot(false);
        return true;
      }
      const event = readEvent(message, id);
      if (!event) return before === undefined;
      if (before !== undefined || (eventType && event.type !== eventType)) return false;
      const fields = [event.type, event.subject_type, event.subject_id, event.actor_id];
      if (!query || fields.some((field) => field.toLowerCase().includes(query.toLowerCase()))) {
        const receivedAt = new Date().toISOString();
        // Keep one extra cursor so Older remains available even if an initially
        // empty Timeline has since received more than a full page of events.
        setLive((previous) => ({
          events: newestFirst([...(previous?.events ?? []), event]).slice(0, PAGE_SIZE + 1),
          receivedAt,
        }));
      }
      return false;
    }), [before, clearSnapshot, eventType, query, readEvent]);
  const resource = useProjectResource(projectId, load, emptyPage, subscribe, before === undefined ? initialDelay : undefined);
  const page = resource.value?.before === before ? resource.value : null;
  const merged = page && before === undefined && live ? newestFirst([...page.items, ...live.events]) : page?.items ?? [];
  const visible = merged.slice(0, PAGE_SIZE);
  const next = merged.length > PAGE_SIZE ? visible.at(-1)?.cursor : page?.next_cursor;
  let state = resource.state;
  if (resource.value && !page) state = "loading";
  if (state === "empty" && visible.length) state = "ready";
  const loadedAt = live && live.receivedAt > (resource.lastLoadedAt ?? "") ? live.receivedAt : resource.lastLoadedAt;
  function showPage(pageCursors: PageCursor[]) {
    if (history.current && history.current.getBoundingClientRect().top < 0) history.current.scrollIntoView();
    setCursors(pageCursors);
  }
  return (
    <div ref={history}>
      <nav className="timeline-pager" aria-label="Timeline pages">
        <button type="button" className="button button--compact button--secondary" disabled={!cursors.length}
          onClick={() => showPage(cursors.slice(0, -1))}>Newer</button>
        <span aria-live="polite">Page {cursors.length + 1}</span>
        <button type="button" className="button button--compact button--secondary" disabled={next == null || state !== "ready"}
          onClick={() => { if (next != null) showPage([...cursors, next]); }}>Older</button>
      </nav>
      {before === undefined ? <Freshness loadedAt={loadedAt} /> : null}
      <StatePanel
        state={state}
        error={resource.error}
        emptyTitle={query || eventType ? "No events match your filters." : "No semantic events"}
        emptyBody={query || eventType ? "Try another type, subject, or actor." : "Meaningful worker and task milestones will appear after an adapter reports real work."}
        onRetry={() => void resource.refresh()}
      >
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
      </StatePanel>
    </div>
  );
}

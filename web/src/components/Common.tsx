import { useEffect, useState, type ReactNode } from "react";
import type { LoadState } from "../hooks/useCollection";
import type { SourceFreshness as SourceFreshnessRecord } from "../api/types";

export function StatePanel({
  state,
  error,
  emptyTitle,
  emptyBody,
  onRetry,
  children,
}: {
  state: LoadState;
  error: string | null;
  emptyTitle: string;
  emptyBody: string;
  onRetry: () => void;
  children: ReactNode;
}) {
  if (state === "loading") {
    return (
      <div className="state-panel" role="status" aria-live="polite">
        <span className="spinner" aria-hidden="true" />
        <div>
          <strong>Loading current state</strong>
          <p>Reading the canonical coordination record.</p>
        </div>
      </div>
    );
  }
  if (state === "denied") {
    return (
      <div className="state-panel state-panel--warning" role="alert">
        <span className="state-icon" aria-hidden="true">⊘</span>
        <div>
          <strong>Access denied</strong>
          <p>Your current role does not permit access to this project data.</p>
        </div>
      </div>
    );
  }
  if (state === "error") {
    return (
      <div className="state-panel state-panel--danger" role="alert">
        <span className="state-icon" aria-hidden="true">!</span>
        <div>
          <strong>Unable to load this view</strong>
          <p>{error ?? "Agent Room returned an unexpected error."}</p>
          <button className="button button--secondary" type="button" onClick={onRetry}>
            Try again
          </button>
        </div>
      </div>
    );
  }
  if (state === "empty") {
    return (
      <div className="state-panel">
        <span className="state-icon" aria-hidden="true">◇</span>
        <div>
          <strong>{emptyTitle}</strong>
          <p>{emptyBody}</p>
        </div>
      </div>
    );
  }
  return <>{children}</>;
}

export function Freshness({ loadedAt }: { loadedAt: string | null }) {
  const [stale, setStale] = useState(false);
  useEffect(() => {
    const check = () => {
      setStale(Boolean(loadedAt && Date.now() - Date.parse(loadedAt) > 60_000));
    };
    check();
    const timer = window.setInterval(check, 15_000);
    return () => window.clearInterval(timer);
  }, [loadedAt]);
  if (!stale) return null;
  return (
    <div className="inline-notice" role="status">
      This view may be stale. Live updates have not refreshed it in over a minute.
    </div>
  );
}

export function PageHeader({
  eyebrow,
  title,
  description,
  actions,
}: {
  eyebrow: string;
  title: string;
  description: string;
  actions?: ReactNode;
}) {
  return (
    <header className="page-header">
      <div>
        <p className="eyebrow">{eyebrow}</p>
        <h1>{title}</h1>
        <p className="page-description">{description}</p>
      </div>
      {actions ? <div className="page-actions">{actions}</div> : null}
    </header>
  );
}

export function StatusPill({ value }: { value: string }) {
  const token = value.toLowerCase().replace(/[^a-z]+/g, "-");
  return <span className={`status-pill status-pill--${token}`}>{value.replaceAll("_", " ")}</span>;
}

export function RelativeTime({ value }: { value: string | null }) {
  const [now, setNow] = useState(Date.now);
  const time = value ? Date.parse(value) : NaN;
  useEffect(() => {
    if (!Number.isFinite(time)) return;
    setNow(Date.now());
    const timer = window.setInterval(() => { setNow(Date.now()); }, 15_000);
    return () => { window.clearInterval(timer); };
  }, [time]);
  if (!value) return <span className="muted">Never</span>;
  if (Number.isNaN(time)) return <time>{value}</time>;
  const seconds = Math.round((time - now) / 1000);
  const formatter = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
  const absolute = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(time);
  let result: string;
  if (Math.abs(seconds) < 60) result = formatter.format(seconds, "second");
  else if (Math.abs(seconds) < 3600) result = formatter.format(Math.round(seconds / 60), "minute");
  else if (Math.abs(seconds) < 86_400) result = formatter.format(Math.round(seconds / 3600), "hour");
  else result = formatter.format(Math.round(seconds / 86_400), "day");
  return <time dateTime={value} title={absolute}>{result}</time>;
}

export function SourceFreshness({ source, compact = false }: { source: SourceFreshnessRecord; compact?: boolean }) {
  const collectedAt = source.last_collected_at ?? source.project_observation?.collected_at ?? null;
  const receivedAt = source.last_received_at ?? source.last_seen;
  const observation = source.project_observation;
  const scanStatus = source.scan_status ?? observation?.scan_status;
  const scanLabel = scanStatus === "complete" ? "Complete collection" : scanStatus === "partial" ? "Partial collection" : scanStatus === "unreadable" ? "Collection unreadable" : "Collection completeness unknown";
  return <span className={`source-freshness${compact ? " source-freshness--compact" : ""}`}>
    <span>{source.system}</span>
    <small>{collectedAt ? <>Source collected <RelativeTime value={collectedAt} /></> : "Collection time not reported"}</small>
    <small>Agent Room received <RelativeTime value={receivedAt} /></small>
    <small className={scanStatus === "partial" || scanStatus === "unreadable" ? "source-freshness__issue" : undefined}>{scanLabel}{scanStatus === "unreadable" ? " · task count unavailable" : observation ? ` · ${observation.task_count} tasks observed` : ""}</small>
    {scanStatus !== "complete" && source.last_successful_collection_at ? <small>Last complete collection <RelativeTime value={source.last_successful_collection_at} /></small> : null}
    {!compact && observation ? <>
      <small>Source project: {observation.name} · {observation.status}</small>
      <small>Source reference: <code>{observation.path}</code></small>
      {observation.scan_error || observation.reason ? <small className={observation.scan_error ? "source-freshness__issue" : undefined}>Source note: {observation.scan_error ?? observation.reason}</small> : null}
    </> : null}
  </span>;
}

export function ActionFeedback({ message, error }: { message: string | null; error: string | null }) {
  return (
    <div className={error ? "action-feedback action-feedback--error" : "action-feedback"} aria-live="polite">
      {error ?? message}
    </div>
  );
}

export function hasCapability(capabilities: string[], capability: string): boolean {
  return capabilities.includes("*") || capabilities.includes(capability);
}

export function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(value);
}

export function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let amount = value / 1024;
  let unit = units[0] ?? "KB";
  for (let index = 1; amount >= 1024 && index < units.length; index += 1) {
    amount /= 1024;
    unit = units[index] ?? unit;
  }
  return `${amount.toFixed(amount >= 10 ? 0 : 1)} ${unit}`;
}

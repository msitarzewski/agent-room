import { useEffect, useState, type ReactNode } from "react";
import type { LoadState } from "../hooks/useCollection";

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
  if (!value) return <span className="muted">Never</span>;
  const time = Date.parse(value);
  if (Number.isNaN(time)) return <time>{value}</time>;
  const seconds = Math.round((time - Date.now()) / 1000);
  const formatter = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
  const absolute = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(time);
  let result: string;
  if (Math.abs(seconds) < 60) result = formatter.format(seconds, "second");
  else if (Math.abs(seconds) < 3600) result = formatter.format(Math.round(seconds / 60), "minute");
  else if (Math.abs(seconds) < 86_400) result = formatter.format(Math.round(seconds / 3600), "hour");
  else result = formatter.format(Math.round(seconds / 86_400), "day");
  return <time dateTime={value} title={absolute}>{result}</time>;
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

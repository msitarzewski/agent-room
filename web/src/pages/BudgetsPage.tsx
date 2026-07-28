import { api } from "../api/client";
import { Freshness, PageHeader, StatePanel, StatusPill, formatNumber } from "../components/Common";
import { useCollection } from "../hooks/useCollection";

export function BudgetsPage() {
  const collection = useCollection(api.budgets);
  return (
    <div className="page">
      <PageHeader
        eyebrow="Resource governance"
        title="Costs & budgets"
        description="Visible limits and actual consumption for cost, tokens, time, turns, concurrency, and tools."
      />
      <Freshness loadedAt={collection.lastLoadedAt} />
      <StatePanel
        state={collection.state}
        error={collection.error}
        emptyTitle="No budgets configured"
        emptyBody="No policy limits exist for this project. An administrator can configure budgets through the protected API."
        onRetry={() => void collection.reload()}
      >
        <div className="budget-grid">
          {collection.items.map((budget) => {
            const measures = [
              { label: "tokens", used: budget.token_used, limit: budget.token_limit },
              { label: "USD", used: budget.cost_used_cents === undefined ? undefined : budget.cost_used_cents / 100, limit: budget.cost_limit_cents === undefined ? undefined : budget.cost_limit_cents / 100 },
              { label: "seconds", used: budget.time_used_seconds, limit: budget.time_limit_seconds },
            ].filter((measure): measure is { label: string; used: number; limit: number } =>
              measure.used !== undefined && measure.limit !== undefined,
            );
            const highestPercent = measures.reduce(
              (highest, measure) => Math.max(highest, measure.limit <= 0 ? 0 : (measure.used / measure.limit) * 100),
              0,
            );
            const status = highestPercent >= 100 ? "exhausted" : highestPercent >= 80 ? "warning" : "healthy";
            return (
              <article className="budget-card" key={budget.id}>
                <div className="card-heading"><div><p className="eyebrow">{budget.scope_type} · {budget.scope_id}</p><h2>{budget.name}</h2></div><StatusPill value={status} /></div>
                {measures.length === 0 ? <p>No quantified limits are set for this budget.</p> : measures.map((measure) => {
                  const percent = measure.limit <= 0 ? 0 : Math.min(100, (measure.used / measure.limit) * 100);
                  return (
                    <div key={measure.label}>
                      <div className="budget-value"><strong>{formatNumber(measure.used)}</strong><span>of {formatNumber(measure.limit)} {measure.label}</span></div>
                      <div className="progress-track" role="progressbar" aria-label={`${budget.name} ${measure.label} consumed`} aria-valuemin={0} aria-valuemax={measure.limit} aria-valuenow={measure.used}>
                        <span style={{ width: `${percent}%` }} />
                      </div>
                      <p>{percent.toFixed(0)}% consumed</p>
                    </div>
                  );
                })}
              </article>
            );
          })}
        </div>
      </StatePanel>
    </div>
  );
}

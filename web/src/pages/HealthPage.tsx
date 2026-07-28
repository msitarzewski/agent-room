import { api } from "../api/client";
import { Freshness, PageHeader, RelativeTime, StatePanel, StatusPill } from "../components/Common";
import { useCollection } from "../hooks/useCollection";

interface VisibleHealthComponent {
  id: string;
  name: string;
  kind: string;
  status: "healthy" | "degraded";
  detail: string;
  checked_at: string;
}

const loadHealth = async (projectId: string, signal?: AbortSignal) => {
  const snapshot = await api.health(projectId, signal);
  const adapters = Object.entries(snapshot.adapters.last_seen);
  const latestAdapter = adapters.sort(([, left], [, right]) => right.localeCompare(left))[0];
  const items: VisibleHealthComponent[] = [
    {
      id: "schema", name: "Database schema", kind: "PostgreSQL",
      status: snapshot.schema.status === "ok" ? "healthy" : "degraded",
      detail: `${snapshot.schema.migrations} migrations verified`,
      checked_at: snapshot.checked_at,
    },
    {
      id: "event-outbox", name: "Event delivery", kind: "outbox",
      status: snapshot.event_outbox.status === "ok" ? "healthy" : "degraded",
      detail: `${snapshot.event_outbox.pending} pending · oldest ${Math.round(snapshot.event_outbox.oldest_seconds)}s`,
      checked_at: snapshot.checked_at,
    },
    {
      id: "control-outbox", name: "Run control delivery", kind: "outbox",
      status: snapshot.control_outbox.status === "ok" ? "healthy" : "degraded",
      detail: `${snapshot.control_outbox.pending} pending`,
      checked_at: snapshot.checked_at,
    },
    {
      id: "adapters", name: "Runtime adapters", kind: "integration",
      status: snapshot.adapters.status === "ok" ? "healthy" : "degraded",
      detail: latestAdapter ? `${adapters.length} observed · latest ${latestAdapter[0]}` : "No adapter events observed",
      checked_at: snapshot.checked_at,
    },
    {
      id: "artifacts", name: "Artifact store", kind: "storage",
      status: snapshot.artifacts.status === "ok" ? "healthy" : "degraded",
      detail: snapshot.artifacts.status === "ok" ? "Content store is writable" : "Content store check failed",
      checked_at: snapshot.checked_at,
    },
    {
      id: "oidc", name: "OIDC", kind: "identity",
      status: snapshot.oidc.status === "ok" ? "healthy" : "degraded",
      detail: snapshot.oidc.configured ? "Provider configured" : "Development authentication mode",
      checked_at: snapshot.checked_at,
    },
    {
      id: "realtime", name: "Realtime stream", kind: "stream",
      status: snapshot.realtime.status === "ok" ? "healthy" : "degraded",
      detail: snapshot.realtime.status === "ok" ? "Hub is available" : "Hub is degraded",
      checked_at: snapshot.checked_at,
    },
  ];
  return { items };
};

export function HealthPage() {
  const collection = useCollection(loadHealth);
  const unhealthy = collection.items.filter((item) => item.status !== "healthy").length;
  return (
    <div className="page">
      <PageHeader
        eyebrow="Operational awareness"
        title="Health & connectivity"
        description="Safe, role-appropriate service and adapter status. Detailed diagnostics remain on private administrative surfaces."
      />
      <Freshness loadedAt={collection.lastLoadedAt} />
      {collection.state === "ready" ? (
        <div className={`health-summary ${unhealthy ? "health-summary--warning" : ""}`} role="status">
          <span aria-hidden="true">{unhealthy ? "△" : "✓"}</span>
          <div><strong>{unhealthy ? `${unhealthy} components need attention` : "All visible components healthy"}</strong>
            <p>Last checks reflect the selected project and your current role.</p></div>
        </div>
      ) : null}
      <StatePanel
        state={collection.state}
        error={collection.error}
        emptyTitle="No visible health components"
        emptyBody="Your role may not expose operational details, or no adapters are configured."
        onRetry={() => void collection.reload()}
      >
        <div className="health-table-wrapper">
          <table>
            <caption className="sr-only">Agent Room component health</caption>
            <thead><tr><th scope="col">Component</th><th scope="col">Type</th><th scope="col">Status</th><th scope="col">Detail</th><th scope="col">Checked</th></tr></thead>
            <tbody>{collection.items.map((component) => (
              <tr key={component.id}>
                <th scope="row">{component.name}</th><td>{component.kind}</td><td><StatusPill value={component.status} /></td>
                <td>{component.detail ?? "No additional detail"}</td><td><RelativeTime value={component.checked_at} /></td>
              </tr>
            ))}</tbody>
          </table>
        </div>
      </StatePanel>
    </div>
  );
}

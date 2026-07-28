import { api } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import {
  Freshness,
  PageHeader,
  RelativeTime,
  StatePanel,
} from "../components/Common";
import { useCollection } from "../hooks/useCollection";

export function EvidencePage() {
  const { projectId } = useAuth();
  const evidence = useCollection(api.evidence);
  const artifacts = useCollection(api.artifacts);
  return (
    <div className="page">
      <PageHeader
        eyebrow="Trust layer"
        title="Evidence & artifacts"
        description="Inspect what supports—or contradicts—worker claims before accepting completion."
      />
      <Freshness loadedAt={evidence.lastLoadedAt} />
      <section aria-labelledby="evidence-heading">
        <div className="section-heading"><h2 id="evidence-heading">Evidence</h2><span>{evidence.items.length} records</span></div>
        <StatePanel
          state={evidence.state}
          error={evidence.error}
          emptyTitle="No evidence captured"
          emptyBody="Completion claims remain unverified until inspectable evidence is attached."
          onRetry={() => void evidence.reload()}
        >
          <div className="card-list">
            {evidence.items.map((item) => (
              <article className="detail-card" key={item.id}>
                <div className="card-heading">
                  <div><p className="eyebrow">{item.kind}</p><h3>{item.summary}</h3></div>
                </div>
                <dl className="metadata-grid">
                  <div><dt>Task</dt><dd><code>{item.task_id ?? "Not linked"}</code></dd></div>
                  <div><dt>Run</dt><dd><code>{item.run_id ?? "Not linked"}</code></dd></div>
                  <div><dt>Captured</dt><dd><RelativeTime value={item.created_at} /></dd></div>
                  <div><dt>Source</dt><dd>{item.source.system}</dd></div>
                  <div><dt>Digest</dt><dd><code>{item.digest ?? "Not supplied"}</code></dd></div>
                  <div className="metadata-wide"><dt>External ID</dt><dd><code>{item.source.external_id ?? "Not supplied"}</code></dd></div>
                </dl>
              </article>
            ))}
          </div>
        </StatePanel>
      </section>
      <section aria-labelledby="artifact-heading">
        <div className="section-heading"><h2 id="artifact-heading">Artifacts</h2><span>{artifacts.items.length} indexed</span></div>
        <StatePanel
          state={artifacts.state}
          error={artifacts.error}
          emptyTitle="No artifacts indexed"
          emptyBody="Protected, content-addressed worker outputs will appear here."
          onRetry={() => void artifacts.reload()}
        >
          <div className="artifact-grid">
            {artifacts.items.map((artifact) => (
              <article className="artifact-card" key={artifact.id}>
                <div className="artifact-icon" aria-hidden="true">◆</div>
                <div>
                  <h3>{artifact.name}</h3><p>{artifact.media_type} · {artifact.source.system}</p>
                  <small>Indexed <RelativeTime value={artifact.created_at} /></small>
                  <code className="digest">{artifact.digest ?? "Digest not supplied"}</code>
                  <code className="digest">{artifact.uri}</code>
                  {projectId ? (
                    <a className="button button--secondary" href={`/api/v1/artifacts/${encodeURIComponent(artifact.id)}/content?${new URLSearchParams({ project_id: projectId }).toString()}`}>
                      Download verified artifact
                    </a>
                  ) : <span className="muted">Select an authorized project to retrieve this artifact.</span>}
                </div>
              </article>
            ))}
          </div>
        </StatePanel>
      </section>
    </div>
  );
}

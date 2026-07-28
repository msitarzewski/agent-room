import type { MouseEvent, ReactNode } from "react";
import { useAuth } from "../auth/AuthContext";
import { useStream } from "../stream/StreamContext";
import { RelativeTime, StatusPill } from "./Common";

const navigation = [
  { to: "/", label: "Attention", glyph: "⌁", end: true },
  { to: "/workers", label: "Workers & runs", glyph: "◉" },
  { to: "/tasks", label: "Tasks & review", glyph: "▦" },
  { to: "/approvals", label: "Approvals", glyph: "✓" },
  { to: "/timeline", label: "Timeline", glyph: "≋" },
  { to: "/evidence", label: "Evidence", glyph: "◇" },
  { to: "/chat", label: "Chat", glyph: "◌" },
  { to: "/budgets", label: "Costs & budgets", glyph: "◒" },
  { to: "/health", label: "Health", glyph: "＋" },
  { to: "/sessions", label: "Sessions", glyph: "◎" },
];

export function AppShell({ children, pathname, navigate }: {
  children: ReactNode;
  pathname: string;
  navigate: (to: string) => void;
}) {
  const { session, projects, projectId, setProjectId, logout } = useAuth();
  const stream = useStream();

  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">Skip to content</a>
      <aside className="sidebar" aria-label="Primary">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">AR</span>
          <div><strong>Agent Room</strong><span>Human control plane</span></div>
        </div>
        <label className="project-picker" htmlFor="project-id">
          <span>Project</span>
          <select id="project-id" value={projectId ?? ""} onChange={(event) => setProjectId(event.target.value)} disabled={projects.length === 0}>
            {projects.length === 0 ? <option value="">No authorized projects</option> : null}
            {projects.map((project) => <option value={project.id} key={project.id}>{project.name}</option>)}
          </select>
        </label>
        <nav aria-label="Primary navigation">
          <ul className="nav-list">
            {navigation.map((item) => (
              <li key={item.to}>
                <a
                  href={item.to}
                  className={pathname === item.to || (!item.end && pathname.startsWith(`${item.to}/`)) ? "active" : undefined}
                  aria-current={pathname === item.to ? "page" : undefined}
                  onClick={(event: MouseEvent<HTMLAnchorElement>) => {
                    if (event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey) {
                      event.preventDefault();
                      navigate(item.to);
                    }
                  }}
                >
                  <span aria-hidden="true">{item.glyph}</span>{item.label}
                </a>
              </li>
            ))}
          </ul>
        </nav>
        <div className="sidebar-footer">
          <div className="identity">
            <span className="avatar">{session?.user.display_name.slice(0, 2).toUpperCase()}</span>
            <div><strong>{session?.user.display_name}</strong><span>{session?.user.username}</span></div>
          </div>
          <button className="text-button" type="button" onClick={() => void logout()}>Sign out</button>
        </div>
      </aside>
      <div className="workspace">
        <header className="topbar">
          <div className="stream-state">
            <span className={`presence-dot presence-dot--${stream.status}`} aria-hidden="true" />
            <span>Live stream</span>
            <StatusPill value={stream.status} />
            {stream.lastMessageAt ? <span className="muted">updated <RelativeTime value={stream.lastMessageAt} /></span> : null}
          </div>
          {stream.status !== "connected" ? (
            <button className="button button--compact button--secondary" type="button" onClick={stream.reconnect}>
              Reconnect
            </button>
          ) : null}
        </header>
        {stream.status === "reconnecting" || stream.status === "error" || stream.status === "disconnected" ? (
          <div className="connection-banner" role="status">
            <strong>Live updates {stream.status}.</strong> Displayed data remains available but may be stale.
          </div>
        ) : null}
        <main id="main-content" tabIndex={-1}>{children}</main>
      </div>
    </div>
  );
}

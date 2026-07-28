import { Component, useEffect, useState, type ErrorInfo, type ReactNode } from "react";
import { useAuth } from "./auth/AuthContext";
import { AppShell } from "./components/AppShell";
import { AttentionPage } from "./pages/AttentionPage";
import { ApprovalsPage } from "./pages/ApprovalsPage";
import { BudgetsPage } from "./pages/BudgetsPage";
import { ChatPage } from "./pages/ChatPage";
import { EvidencePage } from "./pages/EvidencePage";
import { HealthPage } from "./pages/HealthPage";
import { LoginPage } from "./pages/LoginPage";
import { SessionsPage } from "./pages/SessionsPage";
import { TasksPage } from "./pages/TasksPage";
import { TimelinePage } from "./pages/TimelinePage";
import { WorkersPage } from "./pages/WorkersPage";
import { StreamProvider } from "./stream/StreamContext";

class ErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state: { error: Error | null } = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("Agent Room UI failure", error, info.componentStack);
  }

  render() {
    if (this.state.error) {
      return (
        <main className="fatal-error">
          <p className="eyebrow">Interface failure</p>
          <h1>Agent Room could not render this view.</h1>
          <p>{this.state.error.message}</p>
          <button className="button button--primary" type="button" onClick={() => window.location.reload()}>
            Reload the control room
          </button>
        </main>
      );
    }
    return this.props.children;
  }
}

const views: Record<string, ReactNode> = {
  "/": <AttentionPage />,
  "/workers": <WorkersPage />,
  "/tasks": <TasksPage />,
  "/approvals": <ApprovalsPage />,
  "/timeline": <TimelinePage />,
  "/evidence": <EvidencePage />,
  "/chat": <ChatPage />,
  "/budgets": <BudgetsPage />,
  "/health": <HealthPage />,
  "/sessions": <SessionsPage />,
};

function usePathname() {
  const [pathname, setPathname] = useState(window.location.pathname);
  useEffect(() => {
    const update = () => setPathname(window.location.pathname);
    window.addEventListener("popstate", update);
    return () => window.removeEventListener("popstate", update);
  }, []);
  const navigate = (to: string, replace = false) => {
    window.history[replace ? "replaceState" : "pushState"]({}, "", to);
    setPathname(window.location.pathname);
  };
  return { pathname, navigate };
}

function Redirect({ to, navigate }: { to: string; navigate: (to: string, replace?: boolean) => void }) {
  useEffect(() => navigate(to, true), [navigate, to]);
  return null;
}

function ProtectedRoutes({ pathname, navigate }: {
  pathname: string;
  navigate: (to: string, replace?: boolean) => void;
}) {
  const { session, loading, error, refresh } = useAuth();
  if (loading) {
    return <main className="bootstrap-state" role="status"><span className="spinner" aria-hidden="true" /><h1>Opening Agent Room</h1><p>Verifying your session and permissions.</p></main>;
  }
  if (error) {
    return <main className="bootstrap-state" role="alert"><span className="state-icon" aria-hidden="true">!</span><h1>Agent Room is unreachable</h1><p>{error}</p><button className="button button--primary" type="button" onClick={() => void refresh()}>Try again</button></main>;
  }
  if (!session) return <Redirect to="/login" navigate={navigate} />;
  return (
    <StreamProvider>
      <AppShell pathname={pathname} navigate={navigate}>
        {views[pathname] ?? <Redirect to="/" navigate={navigate} />}
      </AppShell>
    </StreamProvider>
  );
}

export function App() {
  const { pathname, navigate } = usePathname();
  return (
    <ErrorBoundary>
      {pathname === "/login" ? <LoginPage navigate={navigate} /> : <ProtectedRoutes pathname={pathname} navigate={navigate} />}
    </ErrorBoundary>
  );
}

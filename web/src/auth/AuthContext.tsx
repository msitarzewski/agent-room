import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { api, ApiError } from "../api/client";
import type { Project, SessionEnvelope } from "../api/types";

interface AuthContextValue {
  session: SessionEnvelope | null;
  loading: boolean;
  error: string | null;
  projects: Project[];
  projectId: string | null;
  login: () => void;
  logout: () => Promise<void>;
  setProjectId: (id: string) => void;
  refresh: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<SessionEnvelope | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectId, setProjectIdState] = useState<string | null>(
    () => localStorage.getItem("agent-room:project"),
  );

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const next = await api.session();
      api.setCsrfToken(next.csrf_token);
      setSession(next);
      const projectPage = await api.projects();
      setProjects(projectPage.items);
      const storedProject = localStorage.getItem("agent-room:project");
      if (!projectPage.items.some((project) => project.id === storedProject)) {
        const firstProject = projectPage.items[0]?.id ?? null;
        setProjectIdState(firstProject);
        if (firstProject) localStorage.setItem("agent-room:project", firstProject);
        else localStorage.removeItem("agent-room:project");
      } else {
        setProjectIdState(storedProject);
      }
    } catch (caught) {
      if (caught instanceof ApiError && caught.status === 401) {
        api.setCsrfToken(null);
        setSession(null);
        setProjects([]);
      } else {
        setError(caught instanceof Error ? caught.message : "Unable to reach Agent Room.");
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const login = useCallback(() => {
    window.location.assign(api.loginUrl(`${window.location.pathname}${window.location.search}`));
  }, []);

  const logout = useCallback(async () => {
    await api.logout();
    api.setCsrfToken(null);
    setSession(null);
    setProjects([]);
    setProjectIdState(null);
    localStorage.removeItem("agent-room:project");
  }, []);

  const setProjectId = useCallback((id: string) => {
    setProjectIdState(id);
    localStorage.setItem("agent-room:project", id);
  }, []);

  const value = useMemo(
    () => ({ session, loading, error, projects, projectId, login, logout, setProjectId, refresh }),
    [session, loading, error, projects, projectId, login, logout, setProjectId, refresh],
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used inside AuthProvider");
  return value;
}

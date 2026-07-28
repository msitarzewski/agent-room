import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import type { StreamMessage, StreamStatus } from "../api/types";
import { useAuth } from "../auth/AuthContext";

interface StreamContextValue {
  status: StreamStatus;
  lastMessageAt: string | null;
  reconnect: () => void;
}

const StreamContext = createContext<StreamContextValue | null>(null);

function streamUrl(projectId: string, cursor: string | null): string {
  const url = new URL("/api/v1/stream", window.location.origin);
  url.protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  url.searchParams.set("project_id", projectId);
  if (cursor) url.searchParams.set("after", cursor);
  return url.toString();
}

export function StreamProvider({ children }: { children: ReactNode }) {
  const { projectId, session } = useAuth();
  const [status, setStatus] = useState<StreamStatus>("disconnected");
  const [lastMessageAt, setLastMessageAt] = useState<string | null>(null);
  const [generation, setGeneration] = useState(0);
  const socketRef = useRef<WebSocket | null>(null);
  const retryRef = useRef(0);
  const timerRef = useRef<number | null>(null);

  const reconnect = useCallback(() => {
    retryRef.current = 0;
    socketRef.current?.close(4000, "Manual reconnect");
    setGeneration((value) => value + 1);
  }, []);

  useEffect(() => {
    if (!session || !projectId) {
      setStatus("disconnected");
      return;
    }
    let disposed = false;
    const cursorKey = `agent-room:cursor:${projectId}`;

    const connect = () => {
      if (disposed) return;
      setStatus(retryRef.current === 0 ? "connecting" : "reconnecting");
      const socket = new WebSocket(streamUrl(projectId, sessionStorage.getItem(cursorKey)));
      socketRef.current = socket;

      socket.addEventListener("open", () => {
        retryRef.current = 0;
        setStatus("connected");
      });
      socket.addEventListener("message", (event) => {
        try {
          const message = JSON.parse(String(event.data)) as StreamMessage;
          if (message.type === "heartbeat") return;
          if (message.type === "resync_required") {
            sessionStorage.removeItem(cursorKey);
          } else if (message.cursor) {
            sessionStorage.setItem(cursorKey, String(message.cursor));
          }
          setLastMessageAt(message.occurred_at);
          window.dispatchEvent(new CustomEvent("agentroom:stream", { detail: message }));
        } catch {
          setStatus("error");
        }
      });
      socket.addEventListener("close", (event) => {
        if (disposed) return;
        if (event.code === 4401 || event.code === 4403) {
          setStatus("error");
          return;
        }
        setStatus("reconnecting");
        const delay = Math.min(30_000, 1_000 * 2 ** retryRef.current);
        retryRef.current += 1;
        timerRef.current = window.setTimeout(connect, delay);
      });
      socket.addEventListener("error", () => {
        setStatus("error");
      });
    };

    connect();
    return () => {
      disposed = true;
      if (timerRef.current !== null) window.clearTimeout(timerRef.current);
      socketRef.current?.close(1000, "Project changed");
      socketRef.current = null;
    };
  }, [generation, projectId, session]);

  const value = useMemo(
    () => ({ status, lastMessageAt, reconnect }),
    [status, lastMessageAt, reconnect],
  );
  return <StreamContext.Provider value={value}>{children}</StreamContext.Provider>;
}

export function useStream(): StreamContextValue {
  const value = useContext(StreamContext);
  if (!value) throw new Error("useStream must be used inside StreamProvider");
  return value;
}

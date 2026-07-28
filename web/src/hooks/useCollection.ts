import { useCallback, useEffect, useState } from "react";
import { ApiError } from "../api/client";
import type { Page } from "../api/types";
import { useAuth } from "../auth/AuthContext";

export type LoadState = "loading" | "ready" | "empty" | "error" | "denied";

export function useCollection<T>(
  loader: (projectId: string, signal?: AbortSignal) => Promise<Page<T>>,
) {
  const { projectId } = useAuth();
  const [items, setItems] = useState<T[]>([]);
  const [state, setState] = useState<LoadState>("loading");
  const [error, setError] = useState<string | null>(null);
  const [lastLoadedAt, setLastLoadedAt] = useState<string | null>(null);
  const load = useCallback(
    async (signal?: AbortSignal) => {
      if (!projectId) {
        setItems([]);
        setState("empty");
        return;
      }
      setError(null);
      try {
        const page = await loader(projectId, signal);
        setItems(page.items);
        setState(page.items.length === 0 ? "empty" : "ready");
        setLastLoadedAt(new Date().toISOString());
      } catch (caught) {
        if (signal?.aborted) return;
        if (caught instanceof ApiError && (caught.status === 401 || caught.status === 403)) {
          setState("denied");
        } else {
          setState("error");
          setError(caught instanceof Error ? caught.message : "Unable to load data.");
        }
      }
    },
    [loader, projectId],
  );

  useEffect(() => {
    const controller = new AbortController();
    setState("loading");
    void load(controller.signal);
    const onStream = () => void load();
    window.addEventListener("agentroom:stream", onStream);
    return () => {
      controller.abort();
      window.removeEventListener("agentroom:stream", onStream);
    };
  }, [load]);

  return { items, state, error, lastLoadedAt, reload: load };
}

import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError } from "../api/client";
import type { Page, PageCursor } from "../api/types";
import { useAuth } from "../auth/AuthContext";
import { subscribeProjectChanges } from "../stream/StreamContext";

export type LoadState = "loading" | "ready" | "empty" | "error" | "denied";

export type CollectionLoader<T> = (
  projectId: string,
  signal?: AbortSignal,
  cursor?: PageCursor,
) => Promise<Page<T>>;

export async function loadAllPages<T>(
  loader: CollectionLoader<T>,
  projectId: string,
  signal?: AbortSignal,
): Promise<T[]> {
  const items: T[] = [];
  const visited = new Set<string>();
  let cursor: PageCursor | undefined;
  do {
    signal?.throwIfAborted();
    const page = await loader(projectId, signal, cursor);
    signal?.throwIfAborted();
    items.push(...page.items);
    const next = page.next_cursor;
    if (next === undefined || next === null || next === "" || next === 0) return items;
    const key = String(next);
    if (visited.has(key)) throw new Error("The server repeated a collection cursor.");
    visited.add(key);
    cursor = next;
  } while (cursor !== undefined);
  return items;
}

interface ProjectResource<T> {
  projectId: string | null;
  value: T | null;
  state: LoadState;
  error: string | null;
  lastLoadedAt: string | null;
  refreshing: boolean;
  stale: boolean;
}

function initialResource<T>(projectId: string | null): ProjectResource<T> {
  return {
    projectId, value: null, state: projectId ? "loading" : "empty", error: null,
    lastLoadedAt: null, refreshing: false, stale: false,
  };
}

/** One request at a time; an event during a request schedules one fresh snapshot. */
export function useProjectResource<T>(
  projectId: string | null,
  loader: (projectId: string, signal: AbortSignal) => Promise<T>,
  isEmpty: (value: T) => boolean,
) {
  const [record, setRecord] = useState(() => initialResource<T>(projectId));
  const command = useRef<{ projectId: string; refresh: () => Promise<void> } | null>(null);

  useEffect(() => {
    setRecord(initialResource<T>(projectId));
    if (!projectId) return;
    let active = true;
    let generation = 0;
    let lastValue: T | null = null;
    let lastLoadedAt: string | null = null;
    let controller: AbortController | null = null;
    let running: Promise<void> | null = null;
    let pending = false;

    const execute = async () => {
      do {
        pending = false;
        const requestGeneration = ++generation;
        controller = new AbortController();
        setRecord((previous) => ({ ...previous, refreshing: true, stale: lastValue !== null }));
        try {
          const value = await loader(projectId, controller.signal);
          if (!active) return;
          if (requestGeneration !== generation) continue;
          lastValue = value;
          lastLoadedAt = new Date().toISOString();
          setRecord({
            projectId, value, state: isEmpty(value) ? "empty" : "ready", error: null,
            lastLoadedAt, refreshing: false, stale: false,
          });
        } catch (caught) {
          if (!active) return;
          if (requestGeneration !== generation) continue;
          const denied = caught instanceof ApiError && [401, 403].includes(caught.status);
          if (denied) lastValue = null;
          setRecord({
            projectId, value: lastValue, state: denied ? "denied" : "error",
            error: caught instanceof Error ? caught.message : "Unable to load data.",
            lastLoadedAt, refreshing: false, stale: lastValue !== null,
          });
        }
      } while (active && pending);
    };
    const refresh = (): Promise<void> => {
      if (!active) return Promise.resolve();
      if (running) {
        pending = true;
        return running;
      }
      running = execute().finally(() => { running = null; });
      return running;
    };
    command.current = { projectId, refresh };
    const unsubscribe = subscribeProjectChanges(projectId, () => { void refresh(); }, () => {
      ++generation;
      pending = false;
      controller?.abort();
      lastValue = null;
      setRecord({
        ...initialResource<T>(projectId), state: "denied",
        error: "Access to this project has expired or been revoked.",
      });
    });
    void refresh();
    return () => {
      active = false;
      controller?.abort();
      unsubscribe();
      command.current = null;
    };
  }, [projectId, loader, isEmpty]);

  const refresh = useCallback((): Promise<void> => {
    const current = command.current;
    return current?.projectId === projectId ? current.refresh() : Promise.resolve();
  }, [projectId]);
  const current = record.projectId === projectId ? record : initialResource<T>(projectId);
  return { ...current, refresh };
}

const emptyCollection = <T,>(items: T[]) => items.length === 0;

export function useCollection<T>(loader: CollectionLoader<T>) {
  const { projectId } = useAuth();
  const load = useCallback((id: string, signal: AbortSignal) => loadAllPages(loader, id, signal), [loader]);
  const resource = useProjectResource(projectId, load, emptyCollection<T>);
  return { ...resource, items: resource.value ?? [], reload: resource.refresh };
}

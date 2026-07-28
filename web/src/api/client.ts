import type {
  Agent,
  Approval,
  Artifact,
  AttentionItem,
  Budget,
  BriefAcknowledgement,
  ChatMessage,
  Claim,
  Evidence,
  HealthSnapshot,
  Page,
  Problem,
  Project,
  ProjectBrief,
  Run,
  SemanticEvent,
  SessionEnvelope,
  Task,
  WorkSession,
} from "./types";

const API_ROOT = "/api/v1";
let csrfToken: string | null = null;

export class ApiError extends Error {
  readonly status: number;
  readonly problem: Problem;

  constructor(problem: Problem) {
    super(problem.detail ?? problem.title);
    this.name = "ApiError";
    this.status = problem.status;
    this.problem = problem;
  }
}

function idempotencyKey(): string {
  return crypto.randomUUID();
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body !== undefined && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  headers.set("Accept", "application/json");
  if (init.method && init.method !== "GET" && init.method !== "HEAD" && csrfToken) {
    headers.set("X-CSRF-Token", csrfToken);
  }
  const response = await fetch(`${API_ROOT}${path}`, {
    ...init,
    headers,
    credentials: "include",
  });

  if (!response.ok) {
    let problem: Problem;
    try {
      problem = (await response.json()) as Problem;
    } catch {
      problem = {
        title: response.statusText || "Request failed",
        status: response.status,
      };
    }
    throw new ApiError({ ...problem, status: response.status });
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return (await response.json()) as T;
}

function projectQuery(projectId: string, extra: Record<string, string> = {}): string {
  const params = new URLSearchParams({ project_id: projectId, ...extra });
  return `?${params.toString()}`;
}

function mutate<T>(
  path: string,
  body: unknown,
  version?: number,
  projectId?: string,
  method: "POST" | "PATCH" | "DELETE" = "POST",
): Promise<T> {
  const headers: Record<string, string> = { "Idempotency-Key": idempotencyKey() };
  if (version !== undefined) headers["If-Match"] = String(version);
  const target = projectId ? `${path}${projectQuery(projectId)}` : path;
  return request<T>(target, { method, headers, body: JSON.stringify(body) });
}

export const api = {
  setCsrfToken: (token: string | null) => {
    csrfToken = token;
  },
  session: () => request<SessionEnvelope>("/auth/session"),
  loginUrl: (returnTo = "/") =>
    `${API_ROOT}/auth/login?${new URLSearchParams({ return_to: returnTo }).toString()}`,
  logout: () => request<void>("/auth/logout", { method: "POST" }),
  projects: async () => {
    const value = await request<Page<Project> | null>("/projects");
    return {
      ...(value ?? {}),
      items: Array.isArray(value?.items) ? value.items : [],
    };
  },
  agents: (projectId: string, signal?: AbortSignal) =>
    request<Page<Agent>>(`/agents${projectQuery(projectId)}`, { signal }),
  runs: (projectId: string, signal?: AbortSignal) =>
    request<Page<Run>>(`/runs${projectQuery(projectId)}`, { signal }),
  tasks: (projectId: string, signal?: AbortSignal) =>
    request<Page<Task>>(`/tasks${projectQuery(projectId)}`, { signal }),
  events: (projectId: string, after?: number, signal?: AbortSignal) =>
    request<Page<SemanticEvent>>(
      `/events${projectQuery(projectId, after ? { cursor: String(after) } : {})}`,
      { signal },
    ),
  attention: (projectId: string, signal?: AbortSignal) =>
    request<Page<AttentionItem>>(`/attention${projectQuery(projectId)}`, { signal }),
  evidence: (projectId: string, signal?: AbortSignal) =>
    request<Page<Evidence>>(`/evidence${projectQuery(projectId)}`, { signal }),
  artifacts: (projectId: string, signal?: AbortSignal) =>
    request<Page<Artifact>>(`/artifacts${projectQuery(projectId)}`, { signal }),
  messages: (projectId: string, signal?: AbortSignal) =>
    request<Page<ChatMessage>>(`/chat/messages${projectQuery(projectId)}`, { signal }),
  budgets: (projectId: string, signal?: AbortSignal) =>
    request<Page<Budget>>(`/budgets${projectQuery(projectId)}`, { signal }),
  health: (projectId: string, signal?: AbortSignal) =>
    request<HealthSnapshot>(`/health/components${projectQuery(projectId)}`, { signal }),
  sessions: (projectId: string, signal?: AbortSignal) =>
    request<Page<WorkSession>>(`/sessions${projectQuery(projectId)}`, { signal }),
  claims: (projectId: string, signal?: AbortSignal) =>
    request<Page<Claim>>(`/claims${projectQuery(projectId)}`, { signal }),
  approvals: (projectId: string, signal?: AbortSignal) =>
    request<Page<Approval>>(`/approvals${projectQuery(projectId)}`, { signal }),
  brief: async (projectId: string, after = 0, signal?: AbortSignal) => {
    const value = await request<
      Omit<ProjectBrief, "event_counts" | "events" | "open_attention" | "pending_approvals" | "recommended_actions"> &
      Partial<Pick<ProjectBrief, "event_counts" | "events" | "open_attention" | "pending_approvals" | "recommended_actions">>
    >(
      `/brief${projectQuery(projectId, after > 0 ? { after: String(after) } : {})}`,
      { signal },
    );
    return {
      ...value,
      event_counts: value.event_counts ?? {},
      events: value.events ?? [],
      open_attention: value.open_attention ?? [],
      pending_approvals: value.pending_approvals ?? [],
      recommended_actions: value.recommended_actions ?? [],
    };
  },
  acknowledgeBrief: (projectId: string, expectedCursor: number, throughCursor: number) =>
    mutate<BriefAcknowledgement>(
      "/brief/acknowledge",
      { expected_cursor: expectedCursor, through_cursor: throughCursor },
      undefined,
      projectId,
    ),
  requestRunApproval: (run: Run, action: string, message = "", expiresInSeconds = 900) =>
    mutate<Approval>(
      "/approvals",
      {
        run_id: run.id,
        action,
        message,
        expected_target_version: run.version,
        expires_in_seconds: expiresInSeconds,
      },
      undefined,
      run.project_id,
    ),
  attentionAction: (item: AttentionItem, action: "acknowledge" | "resolve") =>
    mutate<AttentionItem>(`/attention/${encodeURIComponent(item.id)}/${action}`, {}, item.version, item.project_id),
  runAction: (run: Run, action: string, message?: string, approvalId?: string) =>
    mutate<Run>(
      `/runs/${encodeURIComponent(run.id)}/actions`,
      { action, ...(message ? { message } : {}), ...(approvalId ? { approval_id: approvalId } : {}) },
      run.version,
      run.project_id,
    ),
  transitionTask: (task: Task, status: string, reason?: string) =>
    mutate<Task>(
      `/tasks/${encodeURIComponent(task.id)}/transition`,
      { status, ...(reason ? { reason } : {}) },
      task.version,
      task.project_id,
    ),
  reviewClaim: (claim: Claim, decision: "accepted" | "rejected") => {
    return mutate<Claim>(
      `/claims/${encodeURIComponent(claim.id)}/review`,
      { decision },
      claim.version,
      claim.project_id,
    );
  },
  decideApproval: (approval: Approval, decision: "approved" | "rejected", note?: string) =>
    mutate<Approval>(
      `/approvals/${encodeURIComponent(approval.id)}/decision`,
      { decision, ...(note ? { note } : {}) },
      approval.version,
      approval.project_id,
    ),
  sendMessage: (projectId: string, body: string) =>
    mutate<ChatMessage>("/chat/messages", { body }, undefined, projectId),
};

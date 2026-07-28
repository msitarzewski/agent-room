import type {
  Agent,
  Approval,
  Artifact,
  AttentionItem,
  Budget,
  ChatMessage,
  Claim,
  Evidence,
  HealthSnapshot,
  ProjectBrief,
  Run,
  SemanticEvent,
  SessionEnvelope,
  Task,
  WorkSession,
} from "../api/types";

const base = { version: 1, project_id: "project_alpha", created_at: "2026-07-27T11:00:00Z", updated_at: "2026-07-27T12:00:00Z" };
const source = { system: "codex", external_id: "native_1" };

export const authenticatedSession: SessionEnvelope = {
  user: {
    id: "human_operator",
    username: "operator",
    display_name: "Test Operator",
    capabilities: ["*"],
  },
  expires_at: "2026-07-27T20:00:00Z",
  csrf_token: "test-csrf",
};

export const attention: AttentionItem = {
  ...base,
  id: "attention_1",
  kind: "review",
  title: "Review authentication evidence",
  detail: "Builder submitted a completion claim that requires human review.",
  severity: "high",
  status: "open",
  owner_id: "human_operator",
  resource_type: "task",
  resource_id: "task_1",
};

export const agent: Agent = {
  ...base,
  id: "agent_builder",
  display_name: "Builder",
  kind: "agent",
  status: "active",
  capabilities: ["run:pause", "run:resume", "run:message", "run:redirect", "run:cancel"],
  source,
};

export const run: Run = {
  ...base,
  id: "run_1",
  agent_id: agent.id,
  task_id: "task_1",
  summary: "Build secure session flow",
  status: "working",
  capabilities: ["run:pause", "run:message", "run:redirect", "run:cancel"],
  source,
};

export const task: Task = {
  ...base,
  id: "task_1",
  title: "Build secure session flow",
  description: "Implement the approved OIDC boundary.",
  owner_id: agent.id,
  status: "review",
  priority: "high",
  dependencies: [],
  review_state: "requested",
  approval_state: "pending",
  source,
};

export const semanticEvent: SemanticEvent = {
  id: "event_1",
  cursor: 42,
  project_id: "project_alpha",
  type: "review.requested",
  subject_type: "task",
  subject_id: task.id,
  actor_id: agent.id,
  occurred_at: "2026-07-27T12:00:00Z",
  correlation_id: "correlation_1",
  schema_version: 1,
  payload: { summary: "Builder requested review" },
};

export const evidence: Evidence = {
  ...base,
  id: "evidence_1",
  scope_type: "task",
  task_id: "task_1",
  run_id: "run_1",
  kind: "test_result",
  summary: "Authentication test report",
  digest: "sha256:abc123",
  source,
};

export const artifact: Artifact = {
  ...base,
  id: "artifact_1",
  scope_type: "project",
  name: "test-report.json",
  media_type: "application/json",
  digest: "sha256:abc123",
  uri: "/api/v1/artifacts/artifact_1/content",
  source,
};

export const pipMessage: ChatMessage = {
  ...base,
  id: "message_1",
  author_id: "agent_pip",
  role: "assistant",
  body: "I found one review that needs your attention.",
};

export const budget: Budget = {
  ...base,
  id: "budget_1",
  name: "Daily token budget",
  scope_type: "project",
  scope_id: "project_alpha",
  enforcement_mode: "hard",
  status: "active",
  token_limit: 100000,
  token_used: 38000,
};

export const health: HealthSnapshot = {
  schema: { status: "ok", migrations: 1 },
  event_outbox: { status: "ok", pending: 0, oldest_seconds: 0 },
  control_outbox: { status: "ok", pending: 0 },
  adapters: { status: "ok", last_seen: { codex: "2026-07-27T12:00:00Z" } },
  artifacts: { status: "ok" },
  oidc: { status: "ok", configured: true },
  realtime: { status: "ok" },
  checked_at: "2026-07-27T12:00:00Z",
};

export const workSession: WorkSession = {
  ...base,
  id: "session_1",
  agent_id: "agent_pip",
  status: "active",
  external_ref: "hermes_session_1",
  conversation_ref: "Agent Room release",
  source: { system: "hermes", external_id: "hermes_session_1" },
};

export const claim: Claim = {
  ...base,
  id: "claim_1",
  task_id: "task_1",
  agent_id: "agent_builder",
  status: "pending",
};

export const approval: Approval = {
  ...base,
  id: "approval_cancel_run_1",
  kind: "run_action",
  status: "approved",
  resource_type: "run",
  resource_id: "run_1",
  requested_by: "human_operator",
  reason: "Stop a run that is no longer safe to continue.",
  command_digest: "sha256:abc123",
  command_version: 1,
  expected_target_version: 1,
  expires_at: "2099-07-27T20:00:00Z",
  context: { action: "cancel" },
};

export const projectBrief: ProjectBrief = {
  project_id: "project_alpha",
  from_cursor: 40,
  through_cursor: 42,
  reviewed_cursor: 40,
  event_counts: { "review.requested": 1 },
  events: [semanticEvent],
  open_attention: [attention],
  pending_approvals: [],
  recommended_actions: ["Triage open attention items"],
  generated_at: "2026-07-27T12:00:00Z",
};

export const endpointItems: Record<string, unknown[]> = {
  "/api/v1/projects": [{
    id: "project_alpha",
    name: "Agent Room",
    version: 1,
    updated_at: "2026-07-27T12:00:00Z",
    capabilities: ["*"],
  }],
  "/api/v1/attention": [attention],
  "/api/v1/agents": [agent],
  "/api/v1/runs": [run],
  "/api/v1/tasks": [task],
  "/api/v1/events": [semanticEvent],
  "/api/v1/evidence": [evidence],
  "/api/v1/artifacts": [artifact],
  "/api/v1/chat/messages": [pipMessage],
  "/api/v1/budgets": [budget],
  "/api/v1/sessions": [workSession],
  "/api/v1/claims": [claim],
  "/api/v1/approvals": [approval],
};

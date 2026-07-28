export type Identifier = string;

export interface Versioned {
  id: Identifier;
  version: number;
  project_id: Identifier;
  created_at: string;
  updated_at: string;
}

export interface Project {
  id: Identifier;
  name: string;
  version: number;
  updated_at: string;
  capabilities: string[];
}

export interface SessionEnvelope {
  user: {
    id: Identifier;
    username: string;
    display_name: string;
    capabilities: string[];
  };
  expires_at: string;
  csrf_token: string;
}

export type Presence = "active" | "idle" | "blocked" | "disconnected" | "unknown";
export type RunStatus =
  | "queued"
  | "running"
  | "working"
  | "paused"
  | "blocked"
  | "completed"
  | "failed"
  | "cancelled"
  | "disconnected";
export type TaskStatus =
  | "inbox"
  | "ready"
  | "working"
  | "review"
  | "completed"
  | "archived"
  | "blocked"
  | "cancelled"
  | "failed"
  | "reopened";

export interface Agent extends Versioned {
  display_name: string;
  kind: string;
  status: Presence;
  capabilities: string[];
  source: Source;
}

export interface Run extends Versioned {
  agent_id: Identifier;
  session_id?: Identifier;
  task_id?: Identifier;
  status: RunStatus;
  summary?: string;
  control_status?: "pending" | "executing" | "executed" | "failed";
  control_action?: "pause" | "resume" | "cancel" | "message" | "redirect";
  control_error?: string;
  capabilities: string[];
  source: Source;
}

export interface Task extends Versioned {
  title: string;
  description?: string;
  owner_id?: Identifier;
  status: TaskStatus;
  priority: "low" | "normal" | "high" | "critical";
  dependencies: Identifier[];
  budget_id?: Identifier;
  review_state?: "not_requested" | "requested" | "approved" | "rejected";
  approval_state?: "not_required" | "pending" | "approved" | "rejected";
  blocked_reason?: string;
  source: Source;
}

export interface Claim extends Versioned {
  task_id: Identifier;
  agent_id: Identifier;
  status: string;
  review_note?: string;
  reviewer_id?: Identifier;
}

export interface Approval extends Versioned {
  kind: string;
  status: "pending" | "approved" | "rejected";
  resource_type: string;
  resource_id: Identifier;
  requested_by: Identifier;
  decided_by?: Identifier;
  reason: string;
  decision_note?: string;
  decision_at?: string;
  command_digest: string;
  command_version: number;
  expected_target_version: number;
  expires_at: string;
  consumed_at?: string;
  consumed_by?: Identifier;
  context?: Record<string, unknown>;
}

export interface Source {
  system: string;
  external_id?: string;
  external_url?: string;
}

export interface SemanticEvent {
  id: Identifier;
  cursor: number;
  project_id: Identifier;
  type: string;
  subject_type: string;
  subject_id: Identifier;
  actor_id: Identifier;
  command_id?: Identifier;
  correlation_id?: Identifier;
  causation_id?: Identifier;
  occurred_at: string;
  schema_version: number;
  source_system?: string;
  source_event_id?: string;
  source_sequence?: number;
  payload: unknown;
}

export interface AttentionItem extends Versioned {
  kind: string;
  severity: "low" | "normal" | "high" | "critical";
  title: string;
  detail?: string;
  resource_type?: string;
  resource_id?: Identifier;
  status: "open" | "acknowledged" | "resolved";
  owner_id?: Identifier;
  acknowledged_at?: string;
  resolved_at?: string;
}

export interface Evidence extends Versioned {
  scope_type: "project" | "task" | "run";
  task_id?: Identifier;
  run_id?: Identifier;
  kind: string;
  summary: string;
  source: Source;
  digest?: string;
  data?: unknown;
}

export interface Artifact extends Versioned {
  scope_type: "project" | "task" | "run";
  task_id?: Identifier;
  run_id?: Identifier;
  name: string;
  media_type: string;
  uri: string;
  digest?: string;
  source: Source;
}

export interface ChatMessage extends Versioned {
  session_id?: Identifier;
  run_id?: Identifier;
  author_id: Identifier;
  role: string;
  body: string;
  metadata?: unknown;
}

export interface Budget extends Versioned {
  name: string;
  scope_type: string;
  scope_id: Identifier;
  enforcement_mode: string;
  status: string;
  token_limit?: number;
  token_used?: number;
  cost_limit_cents?: number;
  cost_used_cents?: number;
  time_limit_seconds?: number;
  time_used_seconds?: number;
  concurrent_limit?: number;
  concurrent_used?: number;
}

export type ComponentStatus = "ok" | "degraded";

export interface HealthSnapshot {
  schema: { status: ComponentStatus; migrations: number };
  event_outbox: { status: ComponentStatus; pending: number; oldest_seconds: number };
  control_outbox: { status: ComponentStatus; pending: number };
  adapters: { status: ComponentStatus; last_seen: Record<string, string> };
  artifacts: { status: ComponentStatus };
  oidc: { status: ComponentStatus; configured: boolean };
  realtime: { status: ComponentStatus };
  checked_at: string;
}

export interface ProjectBrief {
  project_id: Identifier;
  from_cursor: number;
  through_cursor: number;
  reviewed_cursor: number;
  event_counts: Record<string, number>;
  events: SemanticEvent[];
  open_attention: AttentionItem[];
  pending_approvals: Approval[];
  recommended_actions: string[];
  generated_at: string;
}

export interface BriefAcknowledgement {
  reviewed_cursor: number;
  replayed: boolean;
}

export interface WorkSession extends Versioned {
  agent_id: Identifier;
  status: string;
  external_ref?: string;
  conversation_ref?: string;
  source: Source;
  last_reconciled_at?: string;
}

export interface Page<T> {
  items: T[];
  next_cursor?: string | number;
}

export interface Problem {
  type?: string;
  title: string;
  status: number;
  detail?: string;
  code?: string;
  field_errors?: Record<string, string>;
}

export type StreamStatus = "connecting" | "connected" | "reconnecting" | "disconnected" | "error";

export interface StreamMessage<T = unknown> {
  cursor: number;
  type: string;
  occurred_at: string;
  data?: T;
}

-- name: GetResource :one
SELECT document
FROM resources
WHERE project_id = $1 AND kind = $2 AND id = $3;

-- name: ResourceHealth :one
SELECT 1::bigint;

-- name: LatestProjectEventCursor :one
SELECT COALESCE(max(cursor), 0)::bigint
FROM events
WHERE project_id = $1;

-- name: GetServiceToken :one
SELECT id, actor_id, project_ids, capabilities
FROM service_tokens
WHERE token_hash = $1
  AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > now());

-- name: GetProjectCapabilities :one
SELECT capabilities
FROM project_memberships
WHERE project_id = $1 AND user_id = $2;

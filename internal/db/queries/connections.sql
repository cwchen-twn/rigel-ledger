-- ---------------------------------------------------------------------------
-- Runner side
-- ---------------------------------------------------------------------------

-- name: UpsertRunnerKey :one
INSERT INTO runner_keys (public_key) VALUES (@public_key)
ON CONFLICT (public_key) DO UPDATE SET retired_at = NULL
RETURNING *;

-- name: RetireRunnerKeysExcept :exec
-- The runner declares every key it still holds; the rest are retired.
UPDATE runner_keys SET retired_at = now() WHERE retired_at IS NULL AND NOT (id = ANY(@keep::BIGINT[]));

-- name: ActiveRunnerKey :one
-- What browsers seal to: the newest key the runner holds.
SELECT * FROM runner_keys WHERE retired_at IS NULL ORDER BY id DESC LIMIT 1;

-- name: UpsertConnector :exec
INSERT INTO runner_connectors (id, name, country, fields) VALUES (@id, @name, @country, @fields)
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, country = EXCLUDED.country, fields = EXCLUDED.fields, updated_at = now();

-- name: ListConnectors :many
SELECT * FROM runner_connectors ORDER BY country, name;

-- name: ClaimJobs :many
-- Due connections, handed to one runner at a time: the claim lapses after 30
-- minutes, so a runner that died mid-run does not strand its jobs.
UPDATE connections c SET claimed_at = now()
WHERE c.id IN (
    SELECT x.id FROM connections x JOIN runner_keys k ON k.id = x.key_id AND k.retired_at IS NULL
    WHERE x.enabled
      AND (x.claimed_at IS NULL OR x.claimed_at < now() - interval '30 minutes')
      AND (x.last_run_at IS NULL
           OR x.last_run_at < now() - make_interval(hours => x.interval_hours::INT)
           OR x.run_requested_at > x.last_run_at)
    ORDER BY x.last_run_at NULLS FIRST, x.id
    LIMIT @lim
    FOR UPDATE OF x SKIP LOCKED
)
RETURNING c.id, c.user_id, c.book_id, c.connector, c.sealed, c.key_id;

-- name: GetClaimedConnection :one
SELECT * FROM connections WHERE id = @id AND claimed_at IS NOT NULL;

-- name: FinishRun :execrows
UPDATE connections SET status = @status, last_error = @last_error, last_run_at = now(), claimed_at = NULL
WHERE id = @id AND claimed_at IS NOT NULL;

-- name: SetConnectionStatus :exec
UPDATE connections SET status = @status WHERE id = @id;

-- name: CreateConnectionChallenge :one
INSERT INTO connection_challenges (connection_id, kind, prompt, image, expires_at)
VALUES (@connection_id, @kind, @prompt, sqlc.narg(image), @expires_at)
RETURNING id, connection_id, kind, prompt, expires_at, created_at;

-- name: TakeChallengeAnswer :one
-- The runner reads an answer once; it is cleared as it is read.
WITH old AS (
    SELECT ch.id, ch.answer_sealed FROM connection_challenges ch
    WHERE ch.id = @id AND ch.connection_id = @connection_id
    FOR UPDATE
)
UPDATE connection_challenges ch SET answer_sealed = NULL
FROM old WHERE ch.id = old.id
RETURNING old.answer_sealed, ch.answered_at, ch.expires_at;

-- ---------------------------------------------------------------------------
-- User side
-- ---------------------------------------------------------------------------

-- name: CreateConnection :one
INSERT INTO connections (user_id, book_id, connector, label, sealed, key_id, interval_hours)
VALUES (@user_id, @book_id, @connector, @label, @sealed, @key_id, @interval_hours)
RETURNING id;

-- name: ListUserConnections :many
-- A person's connections, never with the sealed blob, and with the one open
-- challenge (if any) the runner is waiting on.
SELECT c.id, c.book_id, b.name AS book_name, c.connector, c.label, c.enabled, c.interval_hours,
       c.status, c.last_error, c.last_run_at, c.run_requested_at, c.created_at,
       (k.retired_at IS NOT NULL)::BOOLEAN AS key_retired,
       -- challenge_id 0: nothing is waiting on this person
       coalesce(ch.id, 0)::BIGINT AS challenge_id, coalesce(ch.kind, '')::TEXT AS challenge_kind,
       coalesce(ch.prompt, '')::TEXT AS challenge_prompt, ch.image AS challenge_image,
       coalesce(ch.expires_at, c.created_at)::TIMESTAMPTZ AS challenge_expires_at
FROM connections c
JOIN books b ON b.id = c.book_id
JOIN runner_keys k ON k.id = c.key_id
LEFT JOIN LATERAL (
    SELECT x.id, x.kind, x.prompt, x.image, x.expires_at FROM connection_challenges x
    WHERE x.connection_id = c.id AND x.answered_at IS NULL AND x.expires_at > now()
    ORDER BY x.id DESC LIMIT 1
) ch ON true
WHERE c.user_id = @user_id
ORDER BY c.created_at, c.id;

-- name: GetUserConnection :one
SELECT id, user_id, book_id, connector, label, enabled, interval_hours, status
FROM connections WHERE id = @id AND user_id = @user_id;

-- name: ReplaceCredentials :execrows
UPDATE connections SET sealed = @sealed, key_id = @key_id, status = 'new', last_error = '', updated_at = now()
WHERE id = @id AND user_id = @user_id;

-- name: UpdateConnection :execrows
UPDATE connections SET book_id = @book_id, label = @label, enabled = @enabled, interval_hours = @interval_hours,
    updated_at = now()
WHERE id = @id AND user_id = @user_id;

-- name: RequestRun :execrows
UPDATE connections SET run_requested_at = now() WHERE id = @id AND user_id = @user_id AND enabled;

-- name: DeleteConnection :execrows
DELETE FROM connections WHERE id = @id AND user_id = @user_id;

-- name: AnswerChallenge :execrows
UPDATE connection_challenges ch SET answer_sealed = @answer_sealed, answered_at = now()
FROM connections c
WHERE ch.id = @id AND c.id = ch.connection_id AND c.id = @connection_id AND c.user_id = @user_id
  AND ch.answered_at IS NULL AND ch.expires_at > now();

-- ---------------------------------------------------------------------------
-- Administration
-- ---------------------------------------------------------------------------

-- name: ListRunnerSessions :many
SELECT s.id, s.label, s.created_at, s.last_used_at, s.expires_at, u.username
FROM sessions s JOIN users u ON u.id = s.user_id
WHERE s.kind = 'runner' AND s.expires_at > now()
ORDER BY s.created_at;

-- name: DeleteRunnerSession :execrows
DELETE FROM sessions WHERE id = @id AND kind = 'runner';

-- name: ListRunnerKeys :many
SELECT * FROM runner_keys ORDER BY id DESC;

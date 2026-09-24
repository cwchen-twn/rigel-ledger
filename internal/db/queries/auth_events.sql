-- name: RecordAuthEvent :exec
INSERT INTO auth_events (username, user_id, ip, user_agent, event, failure, detail)
VALUES (@username, sqlc.narg(user_id), sqlc.narg(ip)::inet, @user_agent, @event, @failure, @detail);

-- name: CountFailures :one
-- Failures inside the window for the three throttle keys, and when the
-- oldest of each counted failure leaves the window.
SELECT
    count(*) FILTER (WHERE username = @username AND ip = sqlc.narg(ip)::inet) AS user_ip,
    count(*) FILTER (WHERE ip = sqlc.narg(ip)::inet)                         AS ip,
    count(*) FILTER (WHERE username = @username AND @username::TEXT <> '')   AS username,
    coalesce(min(created_at), now())::TIMESTAMPTZ                            AS oldest
FROM auth_events
WHERE failure
  AND created_at > now() - make_interval(secs => @window_seconds::BIGINT)
  AND (username = @username OR ip = sqlc.narg(ip)::inet);

-- name: ListUserAuthEvents :many
SELECT id, username, user_id, coalesce(host(ip), '')::TEXT AS ip, user_agent, event, failure, detail, created_at
FROM auth_events
WHERE user_id = @user_id
ORDER BY created_at DESC
LIMIT @lim;

-- name: ListAuthEvents :many
SELECT id, username, user_id, coalesce(host(ip), '')::TEXT AS ip, user_agent, event, failure, detail, created_at
FROM auth_events
ORDER BY created_at DESC
LIMIT @lim;

-- name: DeleteOldAuthEvents :execrows
DELETE FROM auth_events WHERE created_at < now() - INTERVAL '180 days';

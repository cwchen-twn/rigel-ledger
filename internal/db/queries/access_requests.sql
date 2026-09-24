-- name: CreateAccessRequest :one
INSERT INTO access_requests (username, email, message, ip)
VALUES (@username, @email, @message, sqlc.narg(ip)::inet)
RETURNING id, username, email, message, coalesce(host(ip), '')::TEXT AS ip, status, decided_by, decided_at, created_at;

-- name: ListAccessRequests :many
SELECT id, username, email, message, coalesce(host(ip), '')::TEXT AS ip, status, decided_by, decided_at, created_at
FROM access_requests
WHERE sqlc.narg(status)::TEXT IS NULL OR status = sqlc.narg(status)::TEXT
ORDER BY created_at DESC
LIMIT 200;

-- name: GetAccessRequest :one
SELECT id, username, email, message, coalesce(host(ip), '')::TEXT AS ip, status, decided_by, decided_at, created_at
FROM access_requests WHERE id = @id;

-- name: DecideAccessRequest :execrows
UPDATE access_requests SET status = @status, decided_by = @decided_by, decided_at = now()
WHERE id = @id AND status = 'pending';

-- name: PendingAccessRequestExists :one
SELECT EXISTS (
    SELECT 1 FROM access_requests
    WHERE status = 'pending' AND (lower(email) = lower(@email) OR username = @username)
);

-- name: ListAdminEmails :many
SELECT email FROM users WHERE is_admin AND is_active AND email <> '';

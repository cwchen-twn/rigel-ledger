DROP TRIGGER IF EXISTS connections_audit ON connections;
DROP FUNCTION IF EXISTS audit_connection();
DROP TABLE IF EXISTS connection_challenges;
DROP TABLE IF EXISTS connections;
DROP TABLE IF EXISTS runner_connectors;
DROP TABLE IF EXISTS runner_keys;
DELETE FROM sessions WHERE kind = 'runner';
ALTER TABLE sessions DROP CONSTRAINT sessions_kind_check;
ALTER TABLE sessions ADD CONSTRAINT sessions_kind_check CHECK (kind IN ('web', 'api', 'token'));

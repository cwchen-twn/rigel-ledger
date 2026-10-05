-- Back to sync modes: everyone is in client mode, and their runner tokens
-- are personal_runner ones again. The server runner starts with nothing.
ALTER TABLE system_settings
    ADD COLUMN default_sync_mode TEXT NOT NULL DEFAULT 'client' CHECK (default_sync_mode IN ('server', 'client'));
ALTER TABLE users
    ADD COLUMN sync_mode TEXT CHECK (sync_mode IN ('server', 'client'));

ALTER TABLE runner_connectors DROP CONSTRAINT runner_connectors_pkey;
CREATE UNIQUE INDEX runner_connectors_owner_id ON runner_connectors ((coalesce(owner_id, 0)), id);
ALTER TABLE runner_connectors ALTER COLUMN owner_id DROP NOT NULL;
ALTER TABLE runner_keys ALTER COLUMN owner_id DROP NOT NULL;

ALTER TABLE sessions DROP CONSTRAINT sessions_kind_check;
ALTER TABLE sessions ADD CONSTRAINT sessions_kind_check
    CHECK (kind IN ('web', 'api', 'token', 'runner', 'personal_runner'));
UPDATE sessions SET kind = 'personal_runner' WHERE kind = 'runner';

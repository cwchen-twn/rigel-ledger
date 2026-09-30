-- P4c-1.5: where a person's sync runs. 'server': the cluster's runner (the
-- owner's choice for themselves). 'client': a runner on the person's own
-- device, logging in from their own address, so many people's bank logins
-- do not all leave from the cluster's one IP. An admin decides per person;
-- NULL follows the instance default.
ALTER TABLE system_settings
    ADD COLUMN default_sync_mode TEXT NOT NULL DEFAULT 'client' CHECK (default_sync_mode IN ('server', 'client'));
ALTER TABLE users
    ADD COLUMN sync_mode TEXT CHECK (sync_mode IN ('server', 'client'));

-- A person's own runner token: the runner API, confined to its owner.
ALTER TABLE sessions DROP CONSTRAINT sessions_kind_check;
ALTER TABLE sessions ADD CONSTRAINT sessions_kind_check
    CHECK (kind IN ('web', 'api', 'token', 'runner', 'personal_runner'));

-- Keys and connectors belong to a runner: NULL is the server runner, a user
-- id is that person's device. A device cannot change what anyone else sees.
ALTER TABLE runner_keys ADD COLUMN owner_id BIGINT REFERENCES users (id) ON DELETE CASCADE;
CREATE INDEX runner_keys_owner ON runner_keys (owner_id);

ALTER TABLE connections DROP CONSTRAINT connections_connector_fkey;
ALTER TABLE runner_connectors DROP CONSTRAINT runner_connectors_pkey;
ALTER TABLE runner_connectors ADD COLUMN owner_id BIGINT REFERENCES users (id) ON DELETE CASCADE;
CREATE UNIQUE INDEX runner_connectors_owner_id ON runner_connectors ((coalesce(owner_id, 0)), id);

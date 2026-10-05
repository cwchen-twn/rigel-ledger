-- One kind of sync runner (decided 2026-10-05): every person, admin or not,
-- links their own, like a self-hosted CI runner. The owner's happens to run
-- next to the app. Sync modes and the server runner go.

-- The server runner's state: connections sealed to its keys cannot be
-- re-sealed to anyone else's runner, so they go with it.
DELETE FROM connections WHERE key_id IN (SELECT id FROM runner_keys WHERE owner_id IS NULL);
DELETE FROM runner_keys WHERE owner_id IS NULL;
DELETE FROM runner_connectors WHERE owner_id IS NULL;
DELETE FROM sessions WHERE kind = 'runner';

-- A person's runner token is now the only runner token.
UPDATE sessions SET kind = 'runner' WHERE kind = 'personal_runner';
ALTER TABLE sessions DROP CONSTRAINT sessions_kind_check;
ALTER TABLE sessions ADD CONSTRAINT sessions_kind_check CHECK (kind IN ('web', 'api', 'token', 'runner'));

-- Every key and connector belongs to a person's runner.
ALTER TABLE runner_keys ALTER COLUMN owner_id SET NOT NULL;
ALTER TABLE runner_connectors ALTER COLUMN owner_id SET NOT NULL;
DROP INDEX runner_connectors_owner_id;
ALTER TABLE runner_connectors ADD PRIMARY KEY (owner_id, id);

ALTER TABLE users DROP COLUMN sync_mode;
ALTER TABLE system_settings DROP COLUMN default_sync_mode;

-- Back to one server runner: device keys, connectors and tokens go, and so
-- do connections that were sealed to a device.
DELETE FROM connections WHERE key_id IN (SELECT id FROM runner_keys WHERE owner_id IS NOT NULL);
DELETE FROM runner_keys WHERE owner_id IS NOT NULL;
DELETE FROM runner_connectors WHERE owner_id IS NOT NULL;
DROP INDEX IF EXISTS runner_connectors_owner_id;
ALTER TABLE runner_connectors DROP COLUMN owner_id;
ALTER TABLE runner_connectors ADD PRIMARY KEY (id);
DELETE FROM connections WHERE connector NOT IN (SELECT id FROM runner_connectors);
ALTER TABLE connections ADD CONSTRAINT connections_connector_fkey FOREIGN KEY (connector) REFERENCES runner_connectors (id);
DROP INDEX IF EXISTS runner_keys_owner;
ALTER TABLE runner_keys DROP COLUMN owner_id;

DELETE FROM sessions WHERE kind = 'personal_runner';
ALTER TABLE sessions DROP CONSTRAINT sessions_kind_check;
ALTER TABLE sessions ADD CONSTRAINT sessions_kind_check CHECK (kind IN ('web', 'api', 'token', 'runner'));

ALTER TABLE users DROP COLUMN sync_mode;
ALTER TABLE system_settings DROP COLUMN default_sync_mode;

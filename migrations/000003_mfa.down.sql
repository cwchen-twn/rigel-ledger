DROP TABLE IF EXISTS auth_challenges;
DROP TABLE IF EXISTS mfa_recovery_codes;
DROP TABLE IF EXISTS webauthn_credentials;
DROP TABLE IF EXISTS mfa_factors;
ALTER TABLE users DROP COLUMN IF EXISTS signin_alerts, DROP COLUMN IF EXISTS webauthn_id;
ALTER TABLE sessions DROP COLUMN IF EXISTS aal;

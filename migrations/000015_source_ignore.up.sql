-- #81: a source account a person sets aside ("don't import"): another
-- source already brings the same account (集保's view of a broker that
-- Shioaji syncs with its cash). Its waiting rows are ignored, and so is
-- every row it sends afterwards; nothing it staged is deleted, so the same
-- source id is never staged again.
ALTER TABLE source_accounts ADD COLUMN ignored BOOLEAN NOT NULL DEFAULT false;

-- P4a: the import core every source feeds (CSV now, the sync runners
-- later). Rows are staged, matched against the books, and only become
-- transactions when a person accepts them in the review queue.

-- A runner's token: a session of its own kind, limited to sending batches
-- (auth.TokenScope). 'api' stays the full-access login for scripts.
ALTER TABLE sessions DROP CONSTRAINT sessions_kind_check;
ALTER TABLE sessions ADD CONSTRAINT sessions_kind_check CHECK (kind IN ('web', 'api', 'token'));

-- An account at an institution, as a source names it, mapped to one ledger
-- account. Unmapped ones wait in the review queue.
CREATE TABLE source_accounts (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    book_id     BIGINT NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    connector   TEXT NOT NULL CHECK (connector ~ '^[a-z0-9][a-z0-9_-]{0,39}$'),
    external_id TEXT NOT NULL CHECK (external_id <> ''),
    label       TEXT NOT NULL DEFAULT '',
    currency    TEXT REFERENCES commodities (code),
    account_id  BIGINT REFERENCES accounts (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (book_id, connector, external_id)
);

CREATE TABLE import_batches (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    book_id    BIGINT NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    connector  TEXT NOT NULL,
    label      TEXT NOT NULL DEFAULT '',
    received   INT NOT NULL DEFAULT 0,
    duplicates INT NOT NULL DEFAULT 0,
    created_by BIGINT REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX import_batches_book ON import_batches (book_id, created_at DESC);

CREATE TABLE import_rows (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    book_id           BIGINT NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    batch_id          BIGINT NOT NULL REFERENCES import_batches (id) ON DELETE CASCADE,
    source_account_id BIGINT NOT NULL REFERENCES source_accounts (id) ON DELETE CASCADE,
    kind              TEXT NOT NULL CHECK (kind IN ('transaction', 'balance')),
    -- "<connector>:<the source's own id>": a re-sync never stages a row twice.
    external_id       TEXT NOT NULL,
    date              DATE NOT NULL,
    -- In the account's currency, signed as the ledger books it on that
    -- account: debit > 0 (money into a bank, a card payment); credit < 0.
    -- For kind=balance, the balance itself (asset > 0, debt < 0).
    amount            NUMERIC(24,8) NOT NULL,
    currency          TEXT NOT NULL REFERENCES commodities (code),
    description       TEXT NOT NULL DEFAULT '',
    counterparty      TEXT NOT NULL DEFAULT '',
    pending           BOOLEAN NOT NULL DEFAULT false,
    raw               JSONB NOT NULL DEFAULT '{}',
    -- What the matcher proposes; the review queue shows it, a person decides.
    --   new       a new transaction against proposed_account_id (a category)
    --   duplicate already in the books: link to match_transaction_id, clear it
    --   clears    the posted form of an uncleared estimate: set its amount, clear it
    --   transfer  the other side is match_row_id (another row): one transfer
    proposal          TEXT NOT NULL DEFAULT 'new' CHECK (proposal IN ('new', 'duplicate', 'clears', 'transfer')),
    proposed_account_id  BIGINT REFERENCES accounts (id) ON DELETE SET NULL,
    match_transaction_id BIGINT REFERENCES transactions (id) ON DELETE SET NULL,
    match_row_id         BIGINT REFERENCES import_rows (id) ON DELETE SET NULL,
    rule_id           BIGINT,
    status            TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'ignored')),
    transaction_id    BIGINT REFERENCES transactions (id) ON DELETE SET NULL,
    decided_by        BIGINT REFERENCES users (id) ON DELETE SET NULL,
    decided_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (book_id, external_id)
);
CREATE INDEX import_rows_queue ON import_rows (book_id, status, date);
CREATE INDEX import_rows_source ON import_rows (source_account_id, date);

-- "Anything whose description contains X goes to category Y."
CREATE TABLE import_rules (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    book_id           BIGINT NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    priority          INT NOT NULL DEFAULT 100,
    -- Case-insensitive substring of description or counterparty.
    pattern           TEXT NOT NULL CHECK (pattern <> ''),
    -- Only rows from this source account, when set.
    source_account_id BIGINT REFERENCES source_accounts (id) ON DELETE CASCADE,
    account_id        BIGINT NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    created_by        BIGINT REFERENCES users (id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX import_rules_book ON import_rules (book_id, priority, id);
ALTER TABLE import_rows ADD CONSTRAINT import_rows_rule_fk
    FOREIGN KEY (rule_id) REFERENCES import_rules (id) ON DELETE SET NULL;

-- What an institution says an account held on a day. Checked against the
-- books, never written into them: a mismatch is drift, shown on the account.
CREATE TABLE balance_assertions (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    book_id    BIGINT NOT NULL REFERENCES books (id) ON DELETE CASCADE,
    account_id BIGINT NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    date       DATE NOT NULL,
    amount     NUMERIC(24,8) NOT NULL,
    source     TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (account_id, date, source)
);

CREATE TRIGGER import_rules_audit AFTER INSERT OR UPDATE OR DELETE ON import_rules
    FOR EACH ROW EXECUTE FUNCTION audit_row();
CREATE TRIGGER source_accounts_audit AFTER INSERT OR UPDATE OR DELETE ON source_accounts
    FOR EACH ROW EXECUTE FUNCTION audit_row();

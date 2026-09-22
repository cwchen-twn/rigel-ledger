-- Make ledger reference tables language-agnostic.
-- Translations now live in the frontend i18n JSON files, keyed by grade code or type ID.

ALTER TABLE ref_ledger_first_grade
    DROP COLUMN type_name_zh,
    DROP COLUMN description_zh,
    RENAME COLUMN description_en TO description;

ALTER TABLE ref_ledger_second_grade
    DROP COLUMN type_name_zh,
    DROP COLUMN description_zh,
    RENAME COLUMN description_en TO description;

ALTER TABLE ref_ledger_third_grade
    DROP COLUMN type_name_zh,
    DROP COLUMN description_zh,
    RENAME COLUMN description_en TO description;

ALTER TABLE ref_ledger_types
    DROP COLUMN type_name_zh,
    DROP COLUMN description_zh,
    RENAME COLUMN description_en TO description;

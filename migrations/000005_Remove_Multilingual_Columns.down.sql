ALTER TABLE ref_ledger_first_grade
    ADD COLUMN type_name_zh   TEXT,
    ADD COLUMN description_zh TEXT,
    RENAME COLUMN description TO description_en;

ALTER TABLE ref_ledger_second_grade
    ADD COLUMN type_name_zh   TEXT,
    ADD COLUMN description_zh TEXT,
    RENAME COLUMN description TO description_en;

ALTER TABLE ref_ledger_third_grade
    ADD COLUMN type_name_zh   TEXT,
    ADD COLUMN description_zh TEXT,
    RENAME COLUMN description TO description_en;

ALTER TABLE ref_ledger_types
    ADD COLUMN type_name_zh   TEXT,
    ADD COLUMN description_zh TEXT,
    RENAME COLUMN description TO description_en;

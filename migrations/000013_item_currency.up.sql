-- #56: an invoice in another currency than the payment it enriches (a EUR
-- invoice on a TWD card): its lines keep their own currency.
ALTER TABLE transaction_items ADD COLUMN currency TEXT REFERENCES commodities (code);

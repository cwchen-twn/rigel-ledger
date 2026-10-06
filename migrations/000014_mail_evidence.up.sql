-- #57: an email is evidence for a payment. Its invoice number, when it
-- names one another source knows (Apple's or Spotify's 電子發票 number),
-- ties it to that invoice exactly; an email that states no amount (an
-- invoice notice, an eSIM order) is an invoice row of amount 0, matched by
-- its seller and day.
ALTER TABLE import_rows ADD COLUMN reference TEXT;

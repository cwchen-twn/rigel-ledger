-- name: InsertAttachment :one
-- Nothing when the book already holds this content: the caller reads its id.
INSERT INTO attachments (book_id, sha256, filename, mime, size, bytes, created_by)
VALUES (@book_id, @sha256, @filename, @mime, @size, @bytes, @created_by)
ON CONFLICT (book_id, sha256) DO NOTHING
RETURNING id;

-- name: FindAttachment :one
SELECT id FROM attachments WHERE book_id = @book_id AND sha256 = @sha256;

-- name: LinkAttachment :exec
INSERT INTO transaction_attachments (book_id, transaction_id, attachment_id, created_by)
VALUES (@book_id, @transaction_id, @attachment_id, @created_by)
ON CONFLICT (transaction_id, attachment_id) DO NOTHING;

-- name: UnlinkAttachment :execrows
-- The file itself goes with its last link (trigger drop_orphan_attachment).
DELETE FROM transaction_attachments
WHERE book_id = @book_id AND transaction_id = @transaction_id AND attachment_id = @attachment_id;

-- name: ListTransactionAttachments :many
-- What the transactions carry, without the bytes.
SELECT l.transaction_id, a.id, a.filename, a.mime, a.size, l.created_at
FROM transaction_attachments l JOIN attachments a ON a.id = l.attachment_id
WHERE l.transaction_id = ANY(@transaction_ids::BIGINT[])
ORDER BY l.id;

-- name: GetAttachmentFile :one
SELECT id, filename, mime, size, bytes FROM attachments WHERE book_id = @book_id AND id = @id;

-- name: SetRowAttachment :exec
-- A staged row's evidence, set once the row is known to be new.
UPDATE import_rows SET attachment_id = @attachment_id WHERE book_id = @book_id AND id = @id;

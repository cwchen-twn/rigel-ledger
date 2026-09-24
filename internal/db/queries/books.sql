-- name: CreateBook :one
INSERT INTO books (name, base_currency, created_by)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetBook :one
SELECT * FROM books WHERE id = $1;

-- name: ListBooksForUser :many
SELECT sqlc.embed(b), m.role
FROM books b
JOIN book_members m ON m.book_id = b.id
WHERE m.user_id = $1
ORDER BY b.name, b.id;

-- name: UpdateBook :one
UPDATE books SET
    name                       = @name,
    lock_date                  = sqlc.narg(lock_date),
    interest_dividend_cf_class = @interest_dividend_cf_class
WHERE id = @id
RETURNING *;

-- name: GetMemberRole :one
SELECT role FROM book_members WHERE book_id = $1 AND user_id = $2;

-- name: AddMember :exec
INSERT INTO book_members (book_id, user_id, role) VALUES ($1, $2, $3);

-- name: UpdateMemberRole :execrows
UPDATE book_members SET role = $3 WHERE book_id = $1 AND user_id = $2;

-- name: RemoveMember :execrows
DELETE FROM book_members WHERE book_id = $1 AND user_id = $2;

-- name: CountOwners :one
SELECT count(*) FROM book_members WHERE book_id = $1 AND role = 'owner';

-- name: ListMembers :many
SELECT u.id AS user_id, u.username, u.display_name, m.role, m.created_at
FROM book_members m
JOIN users u ON u.id = m.user_id
WHERE m.book_id = $1
ORDER BY m.created_at, u.username;

-- name: SetBookBaseCurrency :one
UPDATE books SET base_currency = @base_currency WHERE id = @id RETURNING *;

-- name: DeleteBookTransactions :exec
-- Postings and tag links go with them (ON DELETE CASCADE).
DELETE FROM transactions WHERE book_id = @book_id;

-- name: DetachBookAccounts :exec
-- accounts.parent_id is RESTRICT: flatten the tree before deleting it.
UPDATE accounts SET parent_id = NULL WHERE book_id = @book_id AND parent_id IS NOT NULL;

-- name: DeleteBookAccounts :exec
DELETE FROM accounts WHERE book_id = @book_id;

-- name: DeleteBookTags :exec
DELETE FROM tags WHERE book_id = @book_id;

-- name: DeleteBook :execrows
-- Members go with it; users.default_book_id is set NULL.
DELETE FROM books WHERE id = @id;

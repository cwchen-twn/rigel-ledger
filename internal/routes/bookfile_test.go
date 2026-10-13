package routes

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
)

// Export a book over the API, import the file as bob's new book, and check
// who may do which.
func TestBookFileExportImport(t *testing.T) {
	f := newAPI(t)
	alice, bob := f.browser("alice"), f.browser("bob")

	var book BookDTO
	alice.json("POST", "/api/books", map[string]string{"name": "Family 2026", "base_currency": "TWD"}, 201, &book)
	ids := accountIDs(t, alice, book.ID)
	base := fmt.Sprintf("/api/books/%d", book.ID)
	alice.json("POST", base+"/transactions", map[string]any{
		"date": "2026-09-15", "payee": "PX Mart", "tags": []string{"home"},
		"lines": []map[string]any{{"account_id": ids["groceries"], "amount": "350"}, {"account_id": ids["cash"], "amount": "-350"}},
	}, 201, nil)
	alice.json("POST", base+"/members", map[string]any{"username": "bob", "role": "editor"}, 204, nil)

	// An editor may not export: the file is the whole book.
	if res, _ := bob.do("GET", base+"/export", nil); res.StatusCode != 403 {
		t.Fatalf("editor export = %d", res.StatusCode)
	}
	res, b := alice.do("GET", base+"/export", nil)
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Disposition"), `attachment; filename=Family-2026-`) {
		t.Fatalf("export = %d %v", res.StatusCode, res.Header)
	}
	var file ledger.BookFile
	if err := json.Unmarshal(b, &file); err != nil {
		t.Fatal(err)
	}
	if file.Format != ledger.BookFileFormat || len(file.Transactions) != 1 || file.Transactions[0].Tags[0] != "home" {
		t.Fatalf("file = %+v", file)
	}

	var imp BookImportDTO
	bob.json("POST", "/api/books/import?name=Bob%27s%20copy", file, 201, &imp)
	if imp.Book.Name != "Bob's copy" || imp.Book.Role != "owner" || imp.Transactions != 1 || imp.Book.ID == book.ID {
		t.Fatalf("import = %+v", imp)
	}
	var bal BalancesDTO
	bob.json("GET", fmt.Sprintf("/api/books/%d/balances?as_of=2026-09-30", imp.Book.ID), nil, 200, &bal)
	if bal.ClassTotals["expense"].String() != "350" {
		t.Fatalf("imported balances = %+v", bal.ClassTotals)
	}

	file.Version = 99
	res, b = bob.do("POST", "/api/books/import", file)
	if res.StatusCode != 422 || errorCode(t, b) != "file_version" {
		t.Fatalf("version 99 = %d %s", res.StatusCode, b)
	}
}

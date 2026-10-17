package zlib

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSimilarBooksMapsRecommendations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/eapi/book/13955378/a49630/similar" || r.Method != http.MethodGet {
			t.Fatalf("request = %s %s, want GET /eapi/book/13955378/a49630/similar", r.Method, r.URL.Path)
		}
		if r.Header.Get("remix-userid") != "123" || r.Header.Get("remix-userkey") != "key123" {
			t.Errorf("missing EAPI auth headers: %v", r.Header)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"success": 1,
			"books": []map[string]any{{
				"id": 5592604, "hash": "a8d65c", "isbn": "",
				"url":   "/book/5wpkRjejJy/2001.html",
				"title": "2001：太空漫游", "author": "阿瑟·克拉克",
			}},
		})
	}))
	defer server.Close()

	c := NewClient(WithDomain(server.URL))
	c.SetMode(ClientModeEAPI)
	c.SetCookies(map[string]string{"remix_userid": "123", "remix_userkey": "key123"})

	books, err := c.SimilarBooks("13955378:a49630")
	if err != nil {
		t.Fatalf("SimilarBooks() error = %v", err)
	}
	if len(books) != 1 {
		t.Fatalf("SimilarBooks() returned %d books, want 1", len(books))
	}
	book := books[0]
	if book.ID != "5592604" || book.Hash != "a8d65c" || book.Name != "2001：太空漫游" {
		t.Errorf("mapped fields = %+v", book)
	}
	if len(book.Authors) != 1 || book.Authors[0] != "阿瑟·克拉克" {
		t.Errorf("authors = %v", book.Authors)
	}
	if book.URL != server.URL+"/book/5wpkRjejJy/2001.html" {
		t.Errorf("URL = %q", book.URL)
	}
	if book.DownloadURL != server.URL+"/eapi/book/5592604/a8d65c/file" {
		t.Errorf("DownloadURL = %q", book.DownloadURL)
	}
}

func TestSimilarBooksReportsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"success": 0, "error": "book not found"})
	}))
	defer server.Close()

	c := NewClient(WithDomain(server.URL))
	c.SetMode(ClientModeEAPI)
	c.SetCookies(map[string]string{"remix_userid": "123", "remix_userkey": "key123"})

	if _, err := c.SimilarBooks("1:abc"); err == nil || !strings.Contains(err.Error(), "book not found") {
		t.Fatalf("SimilarBooks() error = %v, want server error through", err)
	}
}

// The similar endpoint is on the same rejection path as the book and file
// endpoints: a daily-limit refusal must surface as ErrDailyLimit, not as an
// opaque failure.
func TestSimilarBooksClassifiesDailyLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"success": 0, "error": "Daily download limit reached"})
	}))
	defer server.Close()

	c := NewClient(WithDomain(server.URL))
	c.SetMode(ClientModeEAPI)
	c.SetCookies(map[string]string{"remix_userid": "123", "remix_userkey": "key123"})

	_, err := c.SimilarBooks("1:abc")
	if !errors.Is(err, ErrDailyLimit) {
		t.Fatalf("SimilarBooks() error = %v, want ErrDailyLimit", err)
	}
}

// Entries without an addressable id:hash must be dropped, not patched up with
// the queried book's id — that fabrication pairs the source id with a foreign
// hash and downloads the wrong file.
func TestSimilarBooksSkipsUnaddressableEntries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"success": 1,
			"books": []map[string]any{
				{"id": nil, "hash": "aaaaaa", "title": "No id", "author": "A"},
				{"id": 5592604, "hash": "", "title": "No hash", "author": "B"},
				{"id": 5592604, "hash": "a8d65c", "title": "Fine", "author": "C"},
			},
		})
	}))
	defer server.Close()

	c := NewClient(WithDomain(server.URL))
	c.SetMode(ClientModeEAPI)
	c.SetCookies(map[string]string{"remix_userid": "123", "remix_userkey": "key123"})

	books, err := c.SimilarBooks("13955378:a49630")
	if err != nil {
		t.Fatalf("SimilarBooks() error = %v", err)
	}
	if len(books) != 1 || books[0].Name != "Fine" {
		t.Fatalf("SimilarBooks() = %+v, want only the addressable entry", books)
	}
	if books[0].ID != "5592604" {
		t.Errorf("ID = %q, must stay the entry's own id", books[0].ID)
	}
}

func TestSimilarBooksValidatesRef(t *testing.T) {
	c := NewClient(WithDomain("https://example.invalid"))
	c.SetMode(ClientModeEAPI)
	c.SetCookies(map[string]string{"remix_userid": "123", "remix_userkey": "key123"})

	if _, err := c.SimilarBooks("no-hash-here"); err == nil || !strings.Contains(err.Error(), "id:hash") {
		t.Fatalf("SimilarBooks() error = %v, want id:hash format error", err)
	}
}

func TestSimilarBooksRequiresLogin(t *testing.T) {
	c := NewClient(WithDomain("https://example.invalid"))
	if _, err := c.SimilarBooks("1:abc"); err != ErrNotLoggedIn {
		t.Fatalf("SimilarBooks() error = %v, want ErrNotLoggedIn", err)
	}
}

// The search and detail envelopes carry series/volume alongside the fields
// search already maps; both must survive into Book.
func TestSearchEAPIMapsSeriesAndVolume(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"success": 1,
			"books": []map[string]any{{
				"id": 7012304, "title": "Mort", "author": "Terry Pratchett",
				"hash": "beef01", "series": "Discworld", "volume": "4",
			}},
			"pagination": map[string]any{"current": 1, "total_pages": 1},
		})
	}))
	defer server.Close()

	c := NewClient(WithDomain(server.URL))
	c.SetMode(ClientModeEAPI)
	c.SetCookies(map[string]string{"remix_userid": "123", "remix_userkey": "key123"})

	result, err := c.Search("discworld", 1, 10, nil)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(result.Books) != 1 {
		t.Fatalf("Search() returned %d books, want 1", len(result.Books))
	}
	if result.Books[0].Series != "Discworld" || result.Books[0].Volume != "4" {
		t.Errorf("series fields = %+v", result.Books[0])
	}
}

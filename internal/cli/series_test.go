package cli

import (
	"errors"
	"testing"

	zlib "github.com/heartleo/zlib"
)

func fakeSeriesSearch(pages map[int]zlib.SearchResult) searchPageFunc {
	return func(query string, page, count int, opts *zlib.SearchOptions) (zlib.SearchResult, error) {
		if result, ok := pages[page]; ok {
			return result, nil
		}
		return zlib.SearchResult{}, errors.New("unexpected page")
	}
}

func TestCollectSameSeriesFiltersAndDedupes(t *testing.T) {
	search := fakeSeriesSearch(map[int]zlib.SearchResult{
		1: {Books: []zlib.Book{
			{ID: "1", Hash: "aa", Name: "Discworld #1", Series: "Discworld"},
			{ID: "2", Hash: "bb", Name: "Unrelated sci-fi"},                              // no series
			{ID: "3", Hash: "cc", Name: "Other series", Series: "Other"},                 // different series
			{ID: "1", Hash: "aa", Name: "Discworld #1 (duplicate)", Series: "discworld"}, // same book, dup across pages
		}, TotalPages: 2},
		2: {Books: []zlib.Book{
			{ID: "4", Hash: "dd", Name: "Discworld #4", Series: "Discworld"},
		}, TotalPages: 2},
	})

	books, err := collectSameSeries(search, zlib.Book{}, "Discworld", seriesSearchPages)
	if err != nil {
		t.Fatalf("collectSameSeries() error = %v", err)
	}
	if len(books) != 2 {
		t.Fatalf("collectSameSeries() returned %d books, want 2: %+v", len(books), books)
	}
	for _, b := range books {
		if b.ID != "1" && b.ID != "4" {
			t.Errorf("unexpected book kept: %+v", b)
		}
	}
	if books[0].Name != "Discworld #1" {
		t.Errorf("duplicate won over original: %+v", books[0])
	}
}

func TestCollectSameSeriesStopsAtLastPage(t *testing.T) {
	var calls int
	search := func(query string, page, count int, opts *zlib.SearchOptions) (zlib.SearchResult, error) {
		calls++
		return zlib.SearchResult{Books: []zlib.Book{
			{ID: "1", Hash: "aa", Name: "Only volume", Series: "Discworld"},
		}, TotalPages: 1}, nil
	}

	if _, err := collectSameSeries(search, zlib.Book{}, "Discworld", seriesSearchPages); err != nil {
		t.Fatalf("collectSameSeries() error = %v", err)
	}
	if calls != 1 {
		t.Errorf("search called %d times, want 1 (must stop at TotalPages)", calls)
	}
}

func TestCollectSameSeriesSurfacesSearchError(t *testing.T) {
	search := func(query string, page, count int, opts *zlib.SearchOptions) (zlib.SearchResult, error) {
		return zlib.SearchResult{}, errors.New("network down")
	}
	if _, err := collectSameSeries(search, zlib.Book{}, "Discworld", seriesSearchPages); err == nil {
		t.Fatal("collectSameSeries() = nil error, want search error through")
	}
}

func TestCollectSameSeriesSortsByVolumeThenYear(t *testing.T) {
	search := fakeSeriesSearch(map[int]zlib.SearchResult{
		1: {Books: []zlib.Book{
			{ID: "3", Hash: "cc", Name: "C", Series: "Discworld", Volume: "10"},
			{ID: "1", Hash: "aa", Name: "A", Series: "Discworld", Volume: "2", Year: "1999"},
			{ID: "2", Hash: "bb", Name: "B", Series: "Discworld", Volume: "", Year: "1999"},
			{ID: "4", Hash: "dd", Name: "D", Series: "Discworld", Volume: "2", Year: "2001"},
		}, TotalPages: 1},
	})

	books, err := collectSameSeries(search, zlib.Book{}, "Discworld", seriesSearchPages)
	if err != nil {
		t.Fatalf("collectSameSeries() error = %v", err)
	}
	var order []string
	for _, b := range books {
		order = append(order, b.Name)
	}
	// Volume 2 first (1999 before 2001), volume 10 next, unknown volume last.
	want := []string{"A", "D", "C", "B"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestCompareSeriesKey(t *testing.T) {
	if compareSeriesKey("3", "10") >= 0 {
		t.Errorf("numeric volumes must sort numerically")
	}
	if compareSeriesKey("", "999999") <= 0 {
		t.Errorf("empty volume must sort after every number")
	}
	if compareSeriesKey("Book 3", "") <= 0 {
		t.Errorf("non-numeric volume must sort after empty")
	}
	if compareSeriesKey("2", "2") != 0 {
		t.Errorf("equal volumes must compare equal")
	}
}

func TestSeriesJSONViewRewritesIDs(t *testing.T) {
	view := seriesJSONView("Discworld", []zlib.Book{
		{ID: "7012304", Hash: "beef01", Name: "Mort"},
	})
	if view.Series != "Discworld" || len(view.Books) != 1 {
		t.Fatalf("view = %+v", view)
	}
	if view.Books[0].ID != "7012304:beef01" {
		t.Errorf("ID = %q, want id:hash for download chaining", view.Books[0].ID)
	}
}

func TestSimilarJSONViewRewritesIDs(t *testing.T) {
	view := similarJSONView([]zlib.Book{
		{ID: "5592604", Hash: "a8d65c", Name: "2001：太空漫游"},
		{ID: "1", Name: "No hash", URL: "https://example.invalid/book/xyz/t.html"},
	})
	if view.Books[0].ID != "5592604:a8d65c" {
		t.Errorf("ID = %q, want id:hash", view.Books[0].ID)
	}
	// Without a hash there is nothing to pair; the alphanumeric URL id takes over.
	if view.Books[1].ID != "xyz" {
		t.Errorf("fallback ID = %q, want URL id", view.Books[1].ID)
	}
}

// The anchor book must appear in its own series listing even when the search
// index never returns it, and must not duplicate itself when it does.
func TestCollectSameSeriesIncludesAnchor(t *testing.T) {
	anchor := zlib.Book{ID: "7012304", Hash: "beef01", Name: "Mort", Series: "Discworld", Volume: "4"}

	search := fakeSeriesSearch(map[int]zlib.SearchResult{
		1: {Books: []zlib.Book{
			// Same id:hash as the anchor: one row, not two.
			{ID: "7012304", Hash: "beef01", Name: "Mort (relisted)", Series: "Discworld"},
			{ID: "880011", Hash: "cafe02", Name: "Other volume", Series: "Discworld"},
		}, TotalPages: 1},
	})

	books, err := collectSameSeries(search, anchor, "Discworld", seriesSearchPages)
	if err != nil {
		t.Fatalf("collectSameSeries() error = %v", err)
	}
	if len(books) != 2 {
		t.Fatalf("collectSameSeries() returned %d books, want 2 (anchor deduped): %+v", len(books), books)
	}

	// Search that misses the anchor entirely: the listing still contains it,
	// so `zlib series <Mort>` can never print a Mort-less Discworld table.
	miss := fakeSeriesSearch(map[int]zlib.SearchResult{
		1: {Books: []zlib.Book{
			{ID: "880011", Hash: "cafe02", Name: "Other volume", Series: "Discworld"},
		}, TotalPages: 1},
	})
	books, err = collectSameSeries(miss, anchor, "Discworld", seriesSearchPages)
	if err != nil {
		t.Fatalf("collectSameSeries() error = %v", err)
	}
	found := false
	for _, b := range books {
		if b.ID == "7012304" {
			found = true
		}
	}
	if !found {
		t.Fatalf("anchor missing from its own series listing: %+v", books)
	}
}

// The empty listing must serialize as "books":[] for jq, never null. The
// guarantee lives in collectSameSeries's initialisation, so that is what this
// pins — a view-level test on a hand-made non-nil literal would prove nothing.
func TestCollectSameSeriesEmptyResultIsNonNil(t *testing.T) {
	search := fakeSeriesSearch(map[int]zlib.SearchResult{
		1: {Books: []zlib.Book{
			{ID: "2", Hash: "bb", Name: "Other series", Series: "Other"},
		}, TotalPages: 1},
	})

	books, err := collectSameSeries(search, zlib.Book{}, "Discworld", seriesSearchPages)
	if err != nil {
		t.Fatalf("collectSameSeries() error = %v", err)
	}
	if books == nil {
		t.Fatal("collectSameSeries() = nil, want empty non-nil slice so --json prints []")
	}
	if len(books) != 0 {
		t.Fatalf("collectSameSeries() = %+v, want empty", books)
	}
}

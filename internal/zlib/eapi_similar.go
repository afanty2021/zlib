package zlib

import (
	"fmt"
	"net/url"
)

// eapiSimilarBook is one entry of /eapi/book/<id>/<hash>/similar. The endpoint
// returns a slimmer card than search: no year, format, size, or scores — just
// enough to identify the book and download it.
type eapiSimilarBook struct {
	ID     any    `json:"id"`
	Hash   string `json:"hash"`
	ISBN   string `json:"isbn"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	Author string `json:"author"`
}

// SimilarBooks returns the server's recommendations for books related to ref
// ("id:hash", the same reference `download` accepts). The suggestions come
// from Z-Library's own similar-books index and carry no metadata beyond
// title/author, but each entry is downloadable directly.
func (c *Client) SimilarBooks(ref string) ([]Book, error) {
	if !c.loggedIn {
		return nil, ErrNotLoggedIn
	}
	id, hash, err := parseEAPIBookRef(ref)
	if err != nil {
		return nil, err
	}
	path := "/eapi/book/" + url.PathEscape(id) + "/" + url.PathEscape(hash) + "/similar"
	body, err := c.eapiGet(path, nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Success int               `json:"success"`
		Error   string            `json:"error"`
		Books   []eapiSimilarBook `json:"books"`
	}
	if err := decodeEAPIJSON(body, &result); err != nil {
		return nil, fmt.Errorf("zlibrary: invalid EAPI similar response: %w", err)
	}
	if result.Success != 1 {
		if result.Error == "" {
			result.Error = "similar request was rejected"
		}
		if isDailyLimitMessage(result.Error) {
			return nil, fmt.Errorf("%w: %s", ErrDailyLimit, result.Error)
		}
		return nil, fmt.Errorf("zlibrary: EAPI similar failed: %s", result.Error)
	}

	books := make([]Book, 0, len(result.Books))
	for _, item := range result.Books {
		id := eapiString(item.ID)
		if id == "" || item.Hash == "" {
			// Without both id and hash there is no reference `download`
			// accepts; an unusable recommendation would only mislead.
			continue
		}
		book := Book{
			ID:      id,
			Hash:    item.Hash,
			ISBN:    item.ISBN,
			URL:     absolutizeURL(c.domain, item.URL),
			Name:    item.Title,
			Authors: nil,
		}
		if item.Author != "" {
			book.Authors = []string{item.Author}
		}
		book.DownloadURL = c.domain + "/eapi/book/" + url.PathEscape(book.ID) + "/" + url.PathEscape(book.Hash) + "/file"
		books = append(books, book)
	}
	return books, nil
}

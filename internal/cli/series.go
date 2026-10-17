package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	zlib "github.com/heartleo/zlib"
	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
)

// seriesSearchPages bounds the search fan-out behind `zlib series`. Five pages
// of fifty hits is far more than any series has legitimately uploaded entries;
// past that the query is matching something else and the rest is noise.
const seriesSearchPages = 5

var seriesCmd = &cobra.Command{
	Use:   "series <book-id>",
	Short: "List the other books in a book's series",
	Long: "List every indexed book in the same series as the given book\n" +
		"(referenced the same way as `zlib download`, id:hash in EAPI mode).\n" +
		"Series metadata is filled in by uploaders and not present on every book;\n" +
		"when it is missing, `zlib similar` is the better next step.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		if err := requireEAPIMode(c, "series"); err != nil {
			return err
		}

		book, err := c.FetchBook(args[0])
		if err != nil {
			return err
		}
		series := strings.TrimSpace(book.Series)
		if series == "" {
			return fmt.Errorf("this book has no series recorded (the uploader did not fill it in); try `zlib similar %s` for same-topic recommendations", args[0])
		}

		books, err := collectSameSeries(c.Search, book, series, seriesSearchPages)
		if err != nil {
			return err
		}

		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			return printJSONTo(cmd.OutOrStdout(), seriesJSONView(series, books))
		}

		if len(books) == 0 {
			fmt.Printf("%s No other books found in series %q\n", colorRedFn(symbolError), series)
			return nil
		}

		fullTitle, _ := cmd.Flags().GetBool("full-title")
		fmt.Printf("%s Series %s · %s\n\n",
			colorGreenFn(symbolSuccess),
			colorBoldFn(series),
			colorFaintFn(fmt.Sprintf("%d books", len(books))),
		)

		rows := make([][]string, 0, len(books))
		for i, b := range books {
			id := bookCommandID(b)
			if id == "" {
				id = "-"
			}
			title := b.Name
			if !fullTitle {
				title = runewidth.Truncate(title, 40, "")
			}
			authors := strings.Join(b.Authors, ", ")
			year := b.Year
			if year == "" || year == "0" {
				year = "-"
			}
			ext := strings.ToUpper(b.Extension)
			if ext == "" {
				ext = "-"
			}
			size := b.Size
			if size == "" {
				size = "-"
			}
			volume := b.Volume
			if volume == "" || volume == "0" {
				volume = "-"
			}
			rows = append(rows, []string{
				fmt.Sprintf("%d", i+1), id,
				title,
				runewidth.Truncate(authors, 20, ""),
				volume, year, ext, size,
			})
		}

		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(currentTheme.Accent)).
			Headers("#", "ID", "Title", "Authors", "Volume", "Year", "Format", "Size").
			Rows(rows...).
			StyleFunc(func(row, col int) lipgloss.Style {
				pad := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
				if row == table.HeaderRow {
					return pad.Bold(true).Foreground(currentTheme.Accent)
				}
				switch col {
				case 1: // ID
					return pad.Foreground(currentTheme.Link)
				case 2: // Title
					return pad.Foreground(currentTheme.Title)
				case 6: // Format
					return pad.Foreground(extLipglossColor(rows[row][6]))
				case 0, 3, 4, 5, 7: // #, Authors, Volume, Year, Size
					return pad.Foreground(currentTheme.Muted)
				}
				return pad
			})

		fmt.Println(t)
		return nil
	},
}

// searchPageFunc is the search call collectSameSeries fans out over; it exists
// so the collection logic is testable without a client.
type searchPageFunc func(query string, page, count int, opts *zlib.SearchOptions) (zlib.SearchResult, error)

// collectSameSeries searches for series and keeps the hits whose own series
// field matches exactly, deduplicated and ordered for shelf reading (volume,
// then year, then title). anchor — the book the command was invoked on — is
// seeded into the candidate set: the search index matches titles far more
// often than series metadata, so without it the queried book itself can be
// absent from its own series listing.
func collectSameSeries(search searchPageFunc, anchor zlib.Book, series string, maxPages int) ([]zlib.Book, error) {
	seen := make(map[string]bool)
	matched := make([]zlib.Book, 0, 1)
	if strings.EqualFold(strings.TrimSpace(anchor.Series), series) {
		seen[anchor.ID+":"+anchor.Hash] = true
		matched = append(matched, anchor)
	}
	for page := 1; page <= maxPages; page++ {
		result, err := search(series, page, 50, nil)
		if err != nil {
			return nil, err
		}
		for _, b := range result.Books {
			if !strings.EqualFold(strings.TrimSpace(b.Series), series) {
				continue
			}
			key := b.ID + ":" + b.Hash
			if seen[key] {
				continue
			}
			seen[key] = true
			matched = append(matched, b)
		}
		if result.TotalPages == 0 || page >= result.TotalPages {
			break
		}
	}

	sort.SliceStable(matched, func(i, j int) bool {
		if c := compareSeriesKey(matched[i].Volume, matched[j].Volume); c != 0 {
			return c < 0
		}
		if c := compareSeriesKey(matched[i].Year, matched[j].Year); c != 0 {
			return c < 0
		}
		return matched[i].Name < matched[j].Name
	})
	return matched, nil
}

// compareSeriesKey orders a volume/year field: numeric values ascending and
// first, then non-numeric strings (empty included) alphabetically at the tail.
func compareSeriesKey(a, b string) int {
	na, aErr := strconv.ParseFloat(strings.TrimSpace(a), 64)
	nb, bErr := strconv.ParseFloat(strings.TrimSpace(b), 64)
	aok, bok := aErr == nil, bErr == nil
	switch {
	case aok && bok:
		switch {
		case na < nb:
			return -1
		case na > nb:
			return 1
		default:
			return 0
		}
	case aok:
		return -1
	case bok:
		return 1
	default:
		return strings.Compare(strings.TrimSpace(a), strings.TrimSpace(b))
	}
}

func seriesJSONView(series string, books []zlib.Book) struct {
	Series string      `json:"series"`
	Books  []zlib.Book `json:"books"`
} {
	view := struct {
		Series string      `json:"series"`
		Books  []zlib.Book `json:"books"`
	}{Series: series, Books: books}
	for i, b := range view.Books {
		if id := bookCommandID(b); id != "" {
			view.Books[i].ID = id
		}
	}
	return view
}

func init() {
	seriesCmd.Flags().Bool("full-title", false, "Do not truncate book titles in the results table")
	seriesCmd.Flags().Bool("json", false, "Output results as JSON")
	rootCmd.AddCommand(seriesCmd)
}

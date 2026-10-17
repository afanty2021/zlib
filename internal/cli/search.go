package cli

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	zlib "github.com/heartleo/zlib"
	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
)

const defaultSearchPageSize = 50

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search books with filters",
	Long:  "Search books by keyword. Without a query, open the search form.",
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOut, _ := cmd.Flags().GetBool("json")

		if len(args) == 0 {
			if jsonOut {
				return fmt.Errorf("--json requires a search query (interactive mode has no JSON output)")
			}
			fullTitle, _ := cmd.Flags().GetBool("full-title")
			opts, err := searchOptsFromFlags(cmd)
			if err != nil {
				return err
			}
			selectedID, err := interactiveSearch(nil, opts, fullTitle)
			if err != nil {
				return err
			}
			if selectedID == "" {
				return nil
			}
			dir, err := destDirFlag(cmd)
			if err != nil {
				return err
			}
			sendToKindle, _ := cmd.Flags().GetBool("send-to-kindle")

			return runDownloadWithRotation(func(c *zlib.Client) downloadModel {
				return newDownloadModel(selectedID, dir, c)
			}, sendToKindle, cmd)
		}

		query := strings.Join(args, " ")
		page, _ := cmd.Flags().GetInt("page")
		count, _ := cmd.Flags().GetInt("count")
		fullTitle, _ := cmd.Flags().GetBool("full-title")
		opts, err := searchOptsFromFlags(cmd)
		if err != nil {
			return err
		}

		result, err := newClient().Search(query, page, count, opts)
		if err != nil {
			return err
		}

		if jsonOut {
			// Keep references accepted by `zlib download` in JSON output.
			for i, b := range result.Books {
				if id := bookCommandID(b); id != "" {
					result.Books[i].ID = id
				}
			}
			return printJSON(result)
		}

		if len(result.Books) == 0 {
			lipgloss.Fprintf(os.Stdout, "%s No results found for %q\n", colorRed(symbolError), query)
			return nil
		}

		lipgloss.Fprintf(os.Stdout, "%s Found %s · Page %d / %d\n\n",
			colorGreen(symbolSuccess),
			colorBold(fmt.Sprintf("%d results", len(result.Books))),
			result.Page, result.TotalPages,
		)

		rows := make([][]string, 0, len(result.Books))
		for i, b := range result.Books {
			id := bookCommandID(b)
			if id == "" {
				id = "-"
			}
			authors := strings.Join(b.Authors, ", ")
			title := b.Name
			if !fullTitle {
				title = runewidth.Truncate(title, 40, "")
			}
			ext := strings.ToUpper(b.Extension)
			if ext == "" {
				ext = "-"
			}
			size := b.Size
			if size == "" {
				size = "-"
			}
			year := b.Year
			if year == "" || year == "0" {
				year = "-"
			}
			rating := b.Rating
			if rating == "" {
				rating = "-"
			}
			rows = append(rows, []string{
				fmt.Sprintf("%d", i+1), id,
				title,
				runewidth.Truncate(authors, 20, ""),
				year, ext, size, rating,
			})
		}

		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(currentTheme.Accent)).
			Headers("#", "ID", "Title", "Authors", "Year", "Format", "Size", "Rating").
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
				case 5: // Format
					return pad.Foreground(extLipglossColor(rows[row][5]))
				case 7: // Rating
					return pad.Foreground(currentTheme.Success)
				case 0, 3, 4, 6: // #, Authors, Year, Size
					return pad.Foreground(currentTheme.Muted)
				}
				return pad
			})

		lipgloss.Fprintln(os.Stdout, t)
		return nil
	},
}

// bookIDFromURL extracts the alphanumeric book ID from a URL like
// https://z-library.sk/book/2RAqApzDRL/title.html
func bookIDFromURL(u string) string {
	parts := strings.Split(u, "/")
	for i, p := range parts {
		if p == "book" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return u
}

func bookCommandID(book zlib.Book) string {
	if book.ID != "" && book.Hash != "" {
		return book.ID + ":" + book.Hash
	}
	id := bookIDFromURL(book.URL)
	if id == book.URL {
		return ""
	}
	return id
}

func historyCommandID(item zlib.DownloadHistoryItem) string {
	if item.ID != "" && item.Hash != "" {
		return item.ID + ":" + item.Hash
	}
	id := bookIDFromURL(item.URL)
	if id == item.URL {
		return ""
	}
	return id
}

func searchOptsFromFlags(cmd *cobra.Command) (*zlib.SearchOptions, error) {
	exts, _ := cmd.Flags().GetStringArray("ext")
	formats, _ := cmd.Flags().GetStringArray("format")
	exts = append(exts, formats...)
	from, _ := cmd.Flags().GetInt("from-year")
	to, _ := cmd.Flags().GetInt("to-year")
	langs, _ := cmd.Flags().GetStringArray("language")
	exact, _ := cmd.Flags().GetBool("exact")
	orderRaw, _ := cmd.Flags().GetString("order")
	if from < 0 || to < 0 || from > 0 && from < 1000 || to > 0 && to < 1000 || from > 9999 || to > 9999 || from > 0 && to > 0 && from > to {
		return nil, fmt.Errorf("invalid year range")
	}
	order := zlib.OrderOption(orderRaw)
	if order != "" {
		valid := false
		for _, choice := range searchSorts {
			if choice.value == order {
				valid = true
				break
			}
		}
		if !valid {
			return nil, fmt.Errorf("invalid search order %q", orderRaw)
		}
	}
	seen := make(map[zlib.Extension]bool)
	opts := &zlib.SearchOptions{FromYear: from, ToYear: to, Exact: exact, Order: order}
	for _, e := range exts {
		ext := zlib.Extension(strings.ToUpper(e))
		if seen[ext] {
			continue
		}
		seen[ext] = true
		opts.Extensions = append(opts.Extensions, ext)
	}
	for _, language := range langs {
		opts.Languages = append(opts.Languages, zlib.Language(strings.ToLower(language)))
	}
	if len(opts.Extensions) == 0 && len(opts.Languages) == 0 && from == 0 && to == 0 && !exact && order == "" {
		return nil, nil
	}
	return opts, nil
}

func init() {
	searchCmd.Flags().IntP("page", "p", 1, "Page number")
	searchCmd.Flags().IntP("count", "n", defaultSearchPageSize, "Results per page")
	addDestDirFlag(searchCmd)
	searchCmd.Flags().Bool("send-to-kindle", false, "Send the downloaded book to Kindle")
	searchCmd.Flags().StringArray("ext", nil, "Filter by file extension, e.g. epub or pdf (repeatable)")
	searchCmd.Flags().StringArray("format", nil, "Alias for --ext")
	searchCmd.Flags().Int("from-year", 0, "Earliest publication year")
	searchCmd.Flags().Int("to-year", 0, "Latest publication year")
	searchCmd.Flags().StringArray("language", nil, "Filter by language (repeatable)")
	searchCmd.Flags().Bool("exact", false, "Match the query exactly")
	searchCmd.Flags().String("order", "", "Sort: popular, bestmatch, date, titleA, title, year, filesize, filesizeA")
	searchCmd.Flags().Bool("full-title", false, "Show full titles in the results table")
	searchCmd.Flags().Bool("json", false, "Print results as JSON (requires a query)")
}

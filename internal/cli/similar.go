package cli

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	zlib "github.com/heartleo/zlib"
	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
)

var similarCmd = &cobra.Command{
	Use:   "similar <book-id>",
	Short: "Show books similar to a book (server recommendations)",
	Long: "Show Z-Library's own recommendations for books related to a book.\n" +
		"The book is referenced the same way as `zlib download` (id:hash in EAPI mode).\n" +
		"Recommendations carry only title and author; feed the ID column to `zlib download`.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := newClient()
		if err := requireEAPIMode(c, "similar"); err != nil {
			return err
		}

		books, err := c.SimilarBooks(args[0])
		if err != nil {
			return err
		}

		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			return printJSONTo(cmd.OutOrStdout(), similarJSONView(books))
		}

		if len(books) == 0 {
			fmt.Printf("%s No similar books found\n", colorRedFn(symbolError))
			return nil
		}

		fullTitle, _ := cmd.Flags().GetBool("full-title")
		fmt.Printf("%s Similar books · %s\n\n",
			colorGreenFn(symbolSuccess),
			colorBoldFn(fmt.Sprintf("%d recommendations", len(books))),
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
			if authors == "" {
				authors = "-"
			}
			rows = append(rows, []string{
				fmt.Sprintf("%d", i+1), id,
				title,
				runewidth.Truncate(authors, 20, ""),
			})
		}

		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(currentTheme.Accent)).
			Headers("#", "ID", "Title", "Authors").
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
				default: // #, Authors
					return pad.Foreground(currentTheme.Muted)
				}
			})

		fmt.Println(t)
		return nil
	},
}

// similarJSONView rewrites IDs into the id:hash form `zlib download` accepts,
// mirroring what `search --json` does for its rows. bookCommandID's URL-id
// fallback is unreachable from SimilarBooks output (entries without a hash
// are dropped upstream) — kept as defense for a future caller feeding raw
// Books into the view.
func similarJSONView(books []zlib.Book) struct {
	Books []zlib.Book `json:"books"`
} {
	view := struct {
		Books []zlib.Book `json:"books"`
	}{Books: books}
	for i, b := range view.Books {
		if id := bookCommandID(b); id != "" {
			view.Books[i].ID = id
		}
	}
	return view
}

func init() {
	similarCmd.Flags().Bool("full-title", false, "Do not truncate book titles in the results table")
	similarCmd.Flags().Bool("json", false, "Output results as JSON")
	rootCmd.AddCommand(similarCmd)
}

// requireEAPIMode rejects the discovery commands when the session runs in HTML
// mode. Their id:hash references and endpoints only exist under EAPI, and the
// HTML failure modes are misleading: series would blame a missing-uploader
// field the HTML parser never reads, and similar would die on ref parsing or
// an /eapi 404 on an HTML domain.
func requireEAPIMode(c *zlib.Client, command string) error {
	if c.Mode() != zlib.ClientModeEAPI {
		return fmt.Errorf("%s requires an EAPI session; log in again with `zlib login --eapi`", command)
	}
	return nil
}

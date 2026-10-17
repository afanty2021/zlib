package cli

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/heartleo/zlib"
	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show download history",
	Long:  "Without flags, browse download history interactively. Use --json to print it, --download to download a book, or --delete to remove an entry.",
	RunE: func(cmd *cobra.Command, args []string) error {
		page, _ := cmd.Flags().GetInt("page")
		downloadID, _ := cmd.Flags().GetString("download")
		dir, err := destDirFlag(cmd)
		if err != nil {
			return err
		}
		sendToKindle, _ := cmd.Flags().GetBool("send-to-kindle")
		formatFilter, _ := cmd.Flags().GetString("format")
		jsonOut, _ := cmd.Flags().GetBool("json")
		deleteID, _ := cmd.Flags().GetString("delete")

		c := newClient()
		if deleteID != "" {
			if err := c.DeleteDownloadHistory(deleteID); err != nil {
				return err
			}
			if jsonOut {
				return printJSON(map[string]any{"success": true, "id": deleteID})
			}
			lipgloss.Fprintln(cmd.OutOrStdout(), "History entry deleted.")
			return nil
		}

		if downloadID != "" {
			item, err := findHistoryItemByBookID(c, downloadID)
			if err != nil {
				return err
			}
			if item.DownloadURL == "" {
				return runDownload(newDownloadModel(historyCommandID(item), dir, c), sendToKindle, cmd)
			}

			bookName := item.Name
			if bookName == "" {
				bookName = downloadID
			}
			return runDownloadWithRotation(func(c *zlib.Client) downloadModel {
				return newDirectDownloadModel(bookName, item.DownloadURL, dir, c)
			}, sendToKindle, cmd)
		}

		// No flags: interactive mode (--json always forces the static path)
		if !cmd.Flags().Changed("page") && formatFilter == "" && !jsonOut && isInteractive() {
			p := tea.NewProgram(newHistorySelectModel(c))
			result, err := p.Run()
			if err != nil {
				return err
			}
			hm := result.(historySelectModel)
			if hm.err != nil {
				return hm.err
			}
			if hm.selected == nil {
				return nil
			}
			item := hm.selected
			if item.DownloadURL == "" {
				return runDownload(newDownloadModel(historyCommandID(*item), dir, c), sendToKindle, cmd)
			}
			bookName := item.Name
			if bookName == "" {
				bookName = bookIDFromURL(item.URL)
			}
			return runDownloadWithRotation(func(c *zlib.Client) downloadModel {
				return newDirectDownloadModel(bookName, item.DownloadURL, dir, c)
			}, sendToKindle, cmd)
		}

		result, err := c.DownloadHistory(page)
		if err != nil {
			return err
		}

		if len(result.Items) == 0 && !jsonOut {
			lipgloss.Fprintf(os.Stdout, "No download history found on page %d.\n", result.Page)
			return nil
		}

		// Collect book IDs that need detail fetch for Extension/Size
		var missingIDs []string
		for _, item := range result.Items {
			if id := historyCommandID(item); id != "" && item.Extension == "" {
				missingIDs = append(missingIDs, id)
			}
		}
		if len(missingIDs) > 0 {
			details := c.FetchBookDetails(missingIDs)
			for i, item := range result.Items {
				id := historyCommandID(item)
				if d, ok := details[id]; ok {
					if result.Items[i].Extension == "" {
						result.Items[i].Extension = d.Extension
					}
					if result.Items[i].Size == "" {
						result.Items[i].Size = d.Size
					}
				}
			}
		}

		if jsonOut {
			// Apply the same format filter as the table, and fill in the
			// alphanumeric ID that `zlib history --download` accepts.
			items := make([]zlib.DownloadHistoryItem, 0, len(result.Items))
			for _, item := range result.Items {
				if formatFilter != "" && !strings.EqualFold(item.Extension, formatFilter) {
					continue
				}
				if id := historyCommandID(item); id != "" {
					item.ID = id
				}
				items = append(items, item)
			}
			result.Items = items
			return printJSON(result)
		}

		lipgloss.Fprintf(os.Stdout, "%s %s\n\n", colorBold("Download history page"), colorBold(fmt.Sprintf("%d", result.Page)))

		rows := make([][]string, 0, len(result.Items))
		for i, item := range result.Items {
			if formatFilter != "" && !strings.EqualFold(item.Extension, formatFilter) {
				continue
			}
			id := historyCommandID(item)
			if id == "" {
				id = "-"
			}
			ext := item.Extension
			if ext == "" {
				ext = "-"
			}
			size := item.Size
			if size == "" {
				size = "-"
			}
			rows = append(rows, []string{
				fmt.Sprintf("%d", i+1), id,
				runewidth.Truncate(item.Name, 50, ""),
				ext, size, item.Date,
			})
		}

		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(currentTheme.Accent)).
			Headers("#", "ID", "Title", "Format", "Size", "Date").
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
				case 3: // Format
					return pad.Foreground(extLipglossColor(rows[row][3]))
				case 0, 4, 5: // #, Size, Date
					return pad.Foreground(currentTheme.Muted)
				}
				return pad
			})

		lipgloss.Fprintln(os.Stdout, t)

		if len(rows) == 0 && formatFilter != "" {
			lipgloss.Fprintf(os.Stdout, "No %s books found on page %d.\n", strings.ToUpper(formatFilter), result.Page)
		}
		return nil
	},
}

func init() {
	historyCmd.Flags().IntP("page", "p", 1, "Page number")
	historyCmd.Flags().StringP("download", "D", "", "Download a book from history by ID")
	addDestDirFlag(historyCmd)
	historyCmd.Flags().Bool("send-to-kindle", false, "Send the downloaded book to Kindle")
	historyCmd.Flags().StringP("format", "f", "", "Filter by file format, e.g. epub")
	historyCmd.Flags().Bool("json", false, "Print history as JSON")
	historyCmd.Flags().String("delete", "", "Delete a history entry on Z-Library by ID or id:hash; local files stay")
	historyCmd.MarkFlagsMutuallyExclusive("delete", "download")
	historyCmd.MarkFlagsMutuallyExclusive("delete", "page")
	historyCmd.MarkFlagsMutuallyExclusive("delete", "format")
	historyCmd.MarkFlagsMutuallyExclusive("delete", "send-to-kindle")
	rootCmd.AddCommand(historyCmd)
}

func findHistoryItemByBookID(c *zlib.Client, bookID string) (zlib.DownloadHistoryItem, error) {
	const maxHistoryPages = 20

	for page := 1; page <= maxHistoryPages; page++ {
		result, err := c.DownloadHistory(page)
		if err != nil {
			return zlib.DownloadHistoryItem{}, fmt.Errorf("failed to load history page %d: %w", page, err)
		}
		if len(result.Items) == 0 {
			break
		}

		for _, item := range result.Items {
			if historyCommandID(item) == bookID {
				return item, nil
			}
		}
	}

	return zlib.DownloadHistoryItem{}, fmt.Errorf("book %s not found in the first %d history pages", bookID, maxHistoryPages)
}

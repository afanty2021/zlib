package cli

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zlib "github.com/heartleo/zlib"
	"github.com/spf13/cobra"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Show account profile and download quota",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadClient()
		if err != nil {
			return err
		}
		jsonOut, _ := cmd.Flags().GetBool("json")
		if isInteractive() && !jsonOut {
			result, err := tea.NewProgram(newProfilePageModel(c)).Run()
			if err != nil {
				return err
			}
			return result.(profilePageModel).err
		}
		profile, err := c.GetProfile()
		if err != nil {
			return err
		}
		recordAccountQuota(profile.DownloadLimit)

		if jsonOut {
			return printJSON(profile)
		}
		lipgloss.Fprintln(cmd.OutOrStdout(), renderProfileCard(profile))
		return nil
	},
}

// Non-interactive output uses the same fields and quota layout as the TUI.
func renderProfileCard(profile zlib.UserProfile) string {
	return renderProfilePage(profile, 80)
}

func init() {
	profileCmd.Flags().Bool("json", false, "Print profile and quota as JSON")
}

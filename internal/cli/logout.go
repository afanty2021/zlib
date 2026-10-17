package cli

import (
	"errors"
	"fmt"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/heartleo/zlib/internal/config"
	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Delete the saved session",
	RunE: func(cmd *cobra.Command, args []string) error {
		// With accounts configured, session.json is a mirror of the active
		// account; deleting it here would strand the account store with no
		// usable session and quietly log out only one of them. Guard on the
		// file's existence rather than a successful parse, so a corrupted
		// store cannot slip past and lose its session.
		if _, statErr := os.Stat(config.AccountsPath()); statErr == nil {
			_, exists, loadErr := config.LoadAccountsOptional()
			if loadErr != nil {
				return fmt.Errorf("%s exists but could not be read (%v); fix or remove the file, then log out again", config.AccountsPath(), loadErr)
			}
			if exists {
				return fmt.Errorf("multiple accounts are configured; remove one with `zlib accounts remove <name>` or all with `zlib accounts clear`")
			}
		}
		removed, err := clearSavedSession()
		if err != nil {
			return err
		}
		if !removed {
			lipgloss.Fprintf(os.Stdout, "Not logged in.\n")
			return nil
		}
		lipgloss.Fprintf(os.Stdout, "%s Logged out.\n", colorFaintFn(symbolSuccess))
		return nil
	},
}

func clearSavedSession() (bool, error) {
	err := os.Remove(config.SessionPath())
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

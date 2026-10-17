package cli

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/heartleo/zlib"
	"github.com/heartleo/zlib/internal/config"
	"github.com/spf13/cobra"
)

var accountsCmd = &cobra.Command{
	Use:   "accounts",
	Short: "Manage multiple Z-Library accounts",
	Long:  "Manage multiple Z-Library accounts. Downloads rotate to the next account that still has daily quota when one hits its limit.",
}

var accountsAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add an account via the login flow or direct remix credentials",
	Long: "Add an account under <name>. Without credential flags this runs the same\n" +
		"login flow as `zlib login`. With --user-id/--user-key it records the\n" +
		"remix_userid/remix_userkey pair directly (the credentials the desktop app\n" +
		"keeps in its config.json); no network request is made.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := cleanAccountName(args[0])
		if err != nil {
			return err
		}
		store, exists, err := config.LoadAccountsOptional()
		if err != nil {
			return fmt.Errorf("failed to load accounts: %w", err)
		}
		if exists && store.Find(name) != nil {
			return fmt.Errorf("account %q already exists; remove it first with `zlib accounts remove %s`", name, name)
		}

		userID, _ := cmd.Flags().GetString("user-id")
		userKey, _ := cmd.Flags().GetString("user-key")
		domain, _ := cmd.Flags().GetString("domain")

		var account config.Account
		if userID != "" || userKey != "" {
			account, err = accountFromRemixCredentials(name, userID, userKey, domain)
			if err != nil {
				return err
			}
		} else {
			email, _ := cmd.Flags().GetString("email")
			password, _ := cmd.Flags().GetString("password")
			cookieFile, _ := cmd.Flags().GetString("cookies")
			session, imported, err := runLoginFlow(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), loginFlowOptions{
				Email:      email,
				Password:   password,
				CookieFile: cookieFile,
				Domain:     domain,
			})
			if errors.Is(err, errLoginCancelled) {
				return nil
			}
			if err != nil {
				return err
			}
			account = config.AccountFromSession(name, session)
			if imported > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "%s Imported %d cookies as account %q.\n", colorGreenFn(symbolSuccess), imported, name)
			}
		}

		if err := activateAccount(&store, account); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s Added account %s (%s, %s) and made it active.\n",
			colorGreenFn(symbolSuccess), name, account.Domain, accountModeLabel(account))
		return nil
	},
}

var accountsImportCmd = &cobra.Command{
	Use:   "import <name>",
	Short: "File the current session.json login under an account name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := cleanAccountName(args[0])
		if err != nil {
			return err
		}
		session, err := config.LoadSession()
		if err != nil || len(session.Cookies) == 0 {
			return errors.New("no current session to import; run `zlib login` first")
		}
		store, exists, err := config.LoadAccountsOptional()
		if err != nil {
			return fmt.Errorf("failed to load accounts: %w", err)
		}
		if exists && store.Find(name) != nil {
			return fmt.Errorf("account %q already exists; remove it first with `zlib accounts remove %s`", name, name)
		}
		account := config.AccountFromSession(name, session)
		if err := activateAccount(&store, account); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s Imported the current session as account %s.\n", colorGreenFn(symbolSuccess), name)
		return nil
	},
}

var accountsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured accounts",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, exists, err := config.LoadAccountsOptional()
		if err != nil {
			return fmt.Errorf("failed to load accounts: %w", err)
		}
		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			// Never emit credentials: remix cookies are long-lived secrets,
			// and --json output is exactly what ends up in CI logs.
			return printJSONTo(cmd.OutOrStdout(), accountsJSONView(&store))
		}
		if !exists || len(store.Accounts) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No accounts configured. Add one with `zlib accounts add <name>` or `zlib accounts import <name>`.")
			return nil
		}

		envDomain := strings.TrimSpace(os.Getenv(zlib.EnvDomain))
		rows := make([][]string, 0, len(store.Accounts))
		for _, account := range store.Accounts {
			active := ""
			if account.Name == store.Active {
				active = "yes"
			}
			remaining := "unknown"
			if account.Remaining != nil {
				remaining = strconv.Itoa(*account.Remaining)
			}
			domain := account.Domain
			if envDomain != "" && envDomain != account.Domain {
				domain = fmt.Sprintf("%s (forced by %s)", domain, zlib.EnvDomain)
			}
			checked := "-"
			if account.CheckedAt != "" {
				checked = account.CheckedAt
			}
			rows = append(rows, []string{account.Name, active, domain, accountModeLabel(account), remaining, checked})
		}

		t := table.New().
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(currentTheme.Accent)).
			Headers("Account", "Active", "Domain", "Mode", "Remaining", "Checked").
			Rows(rows...).
			StyleFunc(func(row, col int) lipgloss.Style {
				pad := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
				if row == table.HeaderRow {
					return pad.Bold(true).Foreground(currentTheme.Accent)
				}
				switch col {
				case 0:
					return pad.Foreground(currentTheme.Muted)
				case 4:
					return pad.Foreground(remainingColor(parseRemainingCell(rows[row][4])))
				default:
					return pad
				}
			})
		fmt.Fprintln(cmd.OutOrStdout(), t)
		return nil
	},
}

var accountsSwitchCmd = &cobra.Command{
	Use:   "switch <name>",
	Short: "Make another account the active one",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := cleanAccountName(args[0])
		if err != nil {
			return err
		}
		store, exists, err := config.LoadAccountsOptional()
		if err != nil {
			return fmt.Errorf("failed to load accounts: %w", err)
		}
		if !exists {
			return errors.New("no accounts configured; add one with `zlib accounts add <name>`")
		}
		account := store.Find(name)
		if account == nil {
			return fmt.Errorf("no account named %q; configured: %s", name, accountNames(&store))
		}
		if err := activateAccount(&store, *account); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s Switched to account %s (%s).\n", colorGreenFn(symbolSuccess), name, account.Domain)
		return nil
	},
}

var accountsRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an account",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := cleanAccountName(args[0])
		if err != nil {
			return err
		}
		store, exists, err := config.LoadAccountsOptional()
		if err != nil {
			return fmt.Errorf("failed to load accounts: %w", err)
		}
		if !exists {
			return errors.New("no accounts configured")
		}
		wasActive := store.Active == name
		if !store.Remove(name) {
			return fmt.Errorf("no account named %q; configured: %s", name, accountNames(&store))
		}

		if wasActive {
			if len(store.Accounts) > 0 {
				next := store.Accounts[0]
				store.Active = next.Name
				if err := config.SaveSession(next.Session()); err != nil {
					return fmt.Errorf("failed to save session: %w", err)
				}
			} else {
				store.Active = ""
				// The removed account was the only one and its cookies are
				// still mirrored in session.json; drop them too.
				if err := os.Remove(config.SessionPath()); err != nil && !os.IsNotExist(err) {
					return err
				}
				if err := os.Remove(config.AccountsPath()); err != nil && !os.IsNotExist(err) {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s Removed account %s.\n", colorGreenFn(symbolSuccess), name)
				fmt.Fprintln(cmd.OutOrStdout(), "No accounts left; rotation is off until you add one again.")
				return nil
			}
		}
		if err := config.SaveAccounts(store); err != nil {
			return fmt.Errorf("failed to save accounts: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "%s Removed account %s.\n", colorGreenFn(symbolSuccess), name)
		if wasActive && store.Active != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "%s Account %s is now active.\n", colorCyanFn(symbolInfo), store.Active)
		}
		return nil
	},
}

var accountsClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Remove every account (keeps the active session login)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := os.Remove(config.AccountsPath()); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s Removed all accounts; the active login stays in session.json.\n", colorGreenFn(symbolSuccess))
		fmt.Fprintf(cmd.OutOrStdout(), "%s Run `zlib logout` to drop that session as well.\n", colorCyanFn(symbolInfo))
		return nil
	},
}

// accountJSONView is the credential-free projection of an account used by
// `accounts list --json`.
type accountJSONView struct {
	Name      string `json:"name"`
	Active    bool   `json:"active"`
	Domain    string `json:"domain"`
	Mode      string `json:"mode,omitempty"`
	Remaining *int   `json:"remaining,omitempty"`
	CheckedAt string `json:"checked_at,omitempty"`
}

// accountsJSONView strips cookies from the store for JSON output; remix
// cookies are long-lived credentials and JSON output is what gets piped into
// CI logs and shell history.
func accountsJSONView(store *config.AccountStore) map[string]any {
	accounts := make([]accountJSONView, 0, len(store.Accounts))
	for _, account := range store.Accounts {
		accounts = append(accounts, accountJSONView{
			Name:      account.Name,
			Active:    account.Name == store.Active,
			Domain:    account.Domain,
			Mode:      accountModeLabel(account),
			Remaining: account.Remaining,
			CheckedAt: account.CheckedAt,
		})
	}
	return map[string]any{"active": store.Active, "accounts": accounts}
}

// cleanAccountName validates an account name from argv. Names live inside
// accounts.json only, but rejecting blanks keeps `accounts` output readable.
func cleanAccountName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", errors.New("account name is required")
	}
	return name, nil
}

// activateAccount files the account under its name, marks it active, and
// mirrors it into session.json so every command — including ones that predate
// multi-account support — uses it.
func activateAccount(store *config.AccountStore, account config.Account) error {
	store.Upsert(account)
	store.Active = account.Name
	if err := config.SaveAccounts(*store); err != nil {
		return fmt.Errorf("failed to save accounts: %w", err)
	}
	if err := config.SaveSession(account.Session()); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}
	return nil
}

// accountFromRemixCredentials builds an account directly from the
// remix_userid/remix_userkey pair — the same pair a login persists, and the
// one the desktop app keeps in its config.json.
func accountFromRemixCredentials(name, userID, userKey, domainFlag string) (config.Account, error) {
	if userID == "" || userKey == "" {
		return config.Account{}, errors.New("both --user-id and --user-key are required")
	}
	domain := strings.TrimSpace(domainFlag)
	if domain == "" {
		domain = strings.TrimSpace(os.Getenv(zlib.EnvDomain))
	}
	if domain == "" {
		domain = zlib.DefaultEAPIDomain
	}
	allowed, err := zlib.ParseAllowedDomain(domain)
	if err != nil {
		return config.Account{}, err
	}
	return config.Account{
		Name:    name,
		Cookies: map[string]string{"remix_userid": userID, "remix_userkey": userKey},
		Domain:  allowed,
		Mode:    string(zlib.ClientModeEAPI),
	}, nil
}

func accountModeLabel(account config.Account) string {
	if account.Mode == string(zlib.ClientModeEAPI) {
		return "eapi"
	}
	return "html"
}

func accountNames(store *config.AccountStore) string {
	names := make([]string, 0, len(store.Accounts))
	for _, account := range store.Accounts {
		names = append(names, account.Name)
	}
	return strings.Join(names, ", ")
}

// parseRemainingCell turns a Remaining column back into the number
// remainingColor thresholds expect; "unknown" reads as healthy.
func parseRemainingCell(cell string) int {
	value, err := strconv.Atoi(cell)
	if err != nil {
		return 99
	}
	return value
}

func init() {
	accountsAddCmd.Flags().String("email", "", "Z-Library email (login flow)")
	accountsAddCmd.Flags().String("password", "", "Z-Library password (login flow)")
	accountsAddCmd.Flags().String("cookies", "", "Import a Netscape-format cookie file (login flow)")
	accountsAddCmd.Flags().String("user-id", "", "remix_userid credential (as found in the desktop app config)")
	accountsAddCmd.Flags().String("user-key", "", "remix_userkey credential (as found in the desktop app config)")
	accountsAddCmd.Flags().String("domain", "", "Account domain (otherwise ZLIB_DOMAIN, or the EAPI default)")
	accountsAddCmd.Flags().Bool("eapi", false, "Use EAPI (now the only mode)")
	accountsAddCmd.Flags().MarkHidden("eapi")
	accountsListCmd.Flags().Bool("json", false, "Output as JSON")
	accountsCmd.AddCommand(accountsAddCmd)
	accountsCmd.AddCommand(accountsImportCmd)
	accountsCmd.AddCommand(accountsListCmd)
	accountsCmd.AddCommand(accountsSwitchCmd)
	accountsCmd.AddCommand(accountsRemoveCmd)
	accountsCmd.AddCommand(accountsClearCmd)
	rootCmd.AddCommand(accountsCmd)
}

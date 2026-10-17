package cli

import (
	"charm.land/lipgloss/v2"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"syscall"

	"github.com/heartleo/zlib"
	"github.com/heartleo/zlib/internal/config"
	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:               "zlib",
	Short:             "Search, download, and send Z-Library books to Kindle",
	Version:           zlib.Version,
	SilenceUsage:      true,
	SilenceErrors:     true,
	CompletionOptions: cobra.CompletionOptions{HiddenDefaultCmd: true},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show the zlib version",
	Run: func(cmd *cobra.Command, args []string) {
		lipgloss.Fprintf(cmd.OutOrStdout(), "%s version %s\n", rootCmd.Name(), zlib.Version)
	},
}

// resolveClientDomain picks the domain an authenticated client should use.
// ZLIB_DOMAIN is current, explicit user intent, so it outranks the domain
// session.json cached back when the user logged in. An empty result means the
// client should keep the default it already resolved.
func resolveClientDomain(envDomain, sessionDomain string) string {
	if domain := strings.TrimSpace(envDomain); domain != "" {
		return domain
	}
	return strings.TrimSpace(sessionDomain)
}

// errNoSessionCookies marks a session payload without credentials, so
// callers can keep the familiar "Not logged in" wording for it.
var errNoSessionCookies = errors.New("no session cookies")

// clientFromSession builds an authenticated client from an explicit session.
// It returns errors instead of exiting so account rotation (which swaps
// sessions mid-command) and tests can react to failures.
func clientFromSession(session config.Session) (*zlib.Client, error) {
	if len(session.Cookies) == 0 {
		return nil, errNoSessionCookies
	}
	c := zlib.NewClient()
	// SetDomain before SetCookies: SetCookies seats the jar against c.domain.
	domain := resolveClientDomain(os.Getenv(zlib.EnvDomain), session.Domain)
	if domain == "" {
		return nil, fmt.Errorf("domain is not configured; use login or config")
	}
	allowedDomain, err := zlib.ParseAllowedDomain(domain)
	if err != nil {
		return nil, fmt.Errorf("invalid domain: %w", err)
	}
	c.SetDomain(allowedDomain)
	c.SetCookies(session.Cookies)
	return c, nil
}

func loadClient() (*zlib.Client, error) {
	session, err := config.LoadSession()
	if err != nil || len(session.Cookies) == 0 {
		return nil, fmt.Errorf("not logged in; use /login or zlib login")
	}
	return clientFromSession(session)
}

// newClient loads session and returns an authenticated client.
func newClient() *zlib.Client {
	c, err := loadClient()
	if err != nil {
		lipgloss.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return c
}

func Execute() {
	if err := loadDotEnv(); err != nil {
		lipgloss.Fprintln(os.Stderr, formatCLIError(err))
		os.Exit(1)
	}
	resolveTheme()
	if err := rootCmd.Execute(); err != nil {
		lipgloss.Fprintln(os.Stderr, formatCLIError(err))
		os.Exit(1)
	}
}

func formatCLIError(err error) string {
	if err == nil {
		return ""
	}

	prefix := colorRed(symbolError + " Error:")
	// SMTP errors already name the server, port and TLS mode; a generic
	// network message would hide whether TLS, auth or the port is wrong.
	if errors.Is(err, zlib.ErrSMTPSend) {
		return prefix + " " + err.Error()
	}
	if msg, ok := classifyNetworkError(err); ok {
		return prefix + " " + msg
	}

	return prefix + " " + err.Error()
}

func classifyNetworkError(err error) (string, bool) {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if timeoutErr, ok := urlErr.Err.(interface{ Timeout() bool }); ok && timeoutErr.Timeout() {
			return "network request timed out. Please check your connection and try again.", true
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "network request timed out. Please check your connection and try again.", true
	}

	switch {
	case errors.Is(err, io.EOF),
		errors.Is(err, syscall.ECONNRESET),
		errors.Is(err, syscall.ECONNABORTED),
		errors.Is(err, syscall.ECONNREFUSED),
		errors.Is(err, syscall.ENETUNREACH),
		errors.Is(err, syscall.EHOSTUNREACH):
		return "network request failed. Please check your connection and try again.", true
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "could not resolve the server address. Please check your network or DNS settings and try again.", true
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "connection reset"),
		strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "connection aborted"),
		strings.Contains(msg, "no such host"),
		strings.Contains(msg, "server misbehaving"),
		strings.Contains(msg, "unexpected eof"),
		strings.Contains(msg, " eof"):
		return "network request failed. Please check your connection and try again.", true
	}

	return "", false
}

func init() {
	runewidth.DefaultCondition.EastAsianWidth = false
	defaultHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd == rootCmd {
			lipgloss.Fprintln(cmd.OutOrStdout(), "\n"+strings.Join(homeWordmark(), "\n")+"\n")
		}
		defaultHelp(cmd, args)
	})
	rootCmd.SetVersionTemplate("{{printf \"%s version %s\\n\" .Name .Version}}")
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(recommendedCmd)
	rootCmd.AddCommand(mostPopularCmd)
	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(profileCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(kindleCmd)
	rootCmd.AddCommand(themeCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(updateCmd)
}

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heartleo/zlib/internal/config"
)

// runAccountsCLI drives the real command tree against the already-isolated
// HOME. Callers must isolateConfigHome (and seed files) before this; doing it
// here instead would let earlier file writes land in the real config dir.
func runAccountsCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return executeRootForTest(t, args...)
}

func seedSession(t *testing.T, userID string) {
	t.Helper()
	// Writes must land in the isolated HOME; isolate first if the test has not.
	isolateConfigHome(t)
	if err := config.SaveSession(config.Session{
		Cookies: map[string]string{"remix_userid": userID, "remix_userkey": "seed-key"},
		Domain:  "https://zlib.bz",
		Mode:    "eapi",
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
}

func TestAccountsImportListSwitchRemove(t *testing.T) {
	seedSession(t, "1")
	out, err := runAccountsCLI(t, "accounts", "import", "main")
	if err != nil || !strings.Contains(out, "main") {
		t.Fatalf("accounts import: out=%q err=%v", out, err)
	}
	store, exists, err := config.LoadAccountsOptional()
	if err != nil || !exists || store.Active != "main" || len(store.Accounts) != 1 {
		t.Fatalf("imported store = %+v exists=%v err=%v", store, exists, err)
	}

	if _, err = runAccountsCLI(t, "accounts", "import", "main"); err == nil {
		t.Fatal("duplicate import should fail")
	}

	out, err = runAccountsCLI(t, "accounts", "add", "backup",
		"--user-id", "2", "--user-key", "k2", "--domain", "https://zlib.bz", "--eapi")
	if err != nil || !strings.Contains(out, "backup") {
		t.Fatalf("accounts add: out=%q err=%v", out, err)
	}

	out, err = runAccountsCLI(t, "accounts", "list")
	if err != nil || !strings.Contains(out, "main") || !strings.Contains(out, "backup") {
		t.Fatalf("accounts list: out=%q err=%v", out, err)
	}

	if _, err = runAccountsCLI(t, "accounts", "switch", "main"); err != nil {
		t.Fatalf("accounts switch: %v", err)
	}
	session, err := config.LoadSession()
	if err != nil || session.Cookies["remix_userid"] != "1" {
		t.Fatalf("after switch, session = %+v err=%v", session, err)
	}

	// Removing the active account promotes the remaining one.
	if _, err = runAccountsCLI(t, "accounts", "remove", "main"); err != nil {
		t.Fatalf("accounts remove: %v", err)
	}
	session, err = config.LoadSession()
	if err != nil || session.Cookies["remix_userid"] != "2" {
		t.Fatalf("after remove, session should mirror backup: %+v err=%v", session, err)
	}
	store, _, err = config.LoadAccountsOptional()
	if err != nil || store.Active != "backup" || len(store.Accounts) != 1 {
		t.Fatalf("store after remove = %+v err=%v", store, err)
	}
}

func TestAccountsRemoveLastAccountAlsoClearsSession(t *testing.T) {
	seedSession(t, "1")
	if _, err := runAccountsCLI(t, "accounts", "import", "solo"); err != nil {
		t.Fatalf("accounts import: %v", err)
	}
	if _, err := runAccountsCLI(t, "accounts", "remove", "solo"); err != nil {
		t.Fatalf("accounts remove: %v", err)
	}
	if _, err := config.LoadSession(); err == nil {
		t.Fatal("removing the last account should drop session.json too")
	}
	if _, exists, _ := config.LoadAccountsOptional(); exists {
		t.Fatal("removing the last account should empty the store")
	}
}

func TestLogoutRefusesWhenAccountsConfigured(t *testing.T) {
	seedSession(t, "1")
	if _, err := runAccountsCLI(t, "accounts", "import", "main"); err != nil {
		t.Fatalf("accounts import: %v", err)
	}
	_, err := runAccountsCLI(t, "logout")
	if err == nil || !strings.Contains(err.Error(), "accounts") {
		t.Fatalf("logout with accounts should refuse, got err=%v", err)
	}
	if _, sessionErr := config.LoadSession(); sessionErr != nil {
		t.Fatalf("session.json should survive the refused logout: %v", sessionErr)
	}
}

func TestAccountsClearKeepsSession(t *testing.T) {
	seedSession(t, "1")
	if _, err := runAccountsCLI(t, "accounts", "import", "main"); err != nil {
		t.Fatalf("accounts import: %v", err)
	}
	if _, err := runAccountsCLI(t, "accounts", "clear"); err != nil {
		t.Fatalf("accounts clear: %v", err)
	}
	if _, exists, _ := config.LoadAccountsOptional(); exists {
		t.Fatal("clear should remove accounts.json")
	}
	if _, err := config.LoadSession(); err != nil {
		t.Fatalf("clear keeps session.json: %v", err)
	}
}

func TestAccountsListJSONHidesCookies(t *testing.T) {
	isolateConfigHome(t)
	store := config.AccountStore{
		Active: "main",
		Accounts: []config.Account{
			{Name: "main", Cookies: map[string]string{"remix_userid": "1", "remix_userkey": "super-secret"}, Domain: "https://zlib.bz", Mode: "eapi"},
		},
	}
	if err := config.SaveAccounts(store); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}

	out, err := runAccountsCLI(t, "accounts", "list", "--json")
	if err != nil {
		t.Fatalf("accounts list --json: %v", err)
	}
	if strings.Contains(out, "super-secret") || strings.Contains(out, "remix_userkey") || strings.Contains(out, "cookies") {
		t.Fatalf("JSON output must not carry credentials: %s", out)
	}

	var view struct {
		Active   string `json:"active"`
		Accounts []struct {
			Name   string `json:"name"`
			Active bool   `json:"active"`
			Domain string `json:"domain"`
			Mode   string `json:"mode"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("JSON output is not the sanitized shape: %v (%s)", err, out)
	}
	if view.Active != "main" || len(view.Accounts) != 1 || view.Accounts[0].Name != "main" ||
		!view.Accounts[0].Active || view.Accounts[0].Domain != "https://zlib.bz" || view.Accounts[0].Mode != "eapi" {
		t.Fatalf("sanitized view = %+v", view)
	}
}

func TestLoginWritesThroughToActiveAccount(t *testing.T) {
	isolateConfigHome(t)
	seedSession(t, "1")
	if _, err := runAccountsCLI(t, "accounts", "import", "main"); err != nil {
		t.Fatalf("accounts import: %v", err)
	}

	// A fresh login (cookie import flavor: no network needed) must update the
	// stored credentials of the active account, or rotation would keep
	// replaying the stale cookies.
	cookies := "# Netscape HTTP Cookie File\n" +
		".zlib.bz\tTRUE\t/\tTRUE\t1950000000\tremix_userid\t77\n" +
		".zlib.bz\tTRUE\t/\tTRUE\t1950000000\tremix_userkey\tfresh-key\n"
	cookieFile := filepath.Join(t.TempDir(), "cookies.txt")
	if err := os.WriteFile(cookieFile, []byte(cookies), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	out, err := runAccountsCLI(t, "login", "--cookies", cookieFile, "--domain", "https://zlib.bz")
	if err != nil {
		t.Fatalf("login --cookies: %v (out=%s)", err, out)
	}
	if !strings.Contains(out, "active account main") {
		t.Fatalf("login should report the write-through, got: %s", out)
	}

	store, exists, err := config.LoadAccountsOptional()
	if err != nil || !exists {
		t.Fatalf("LoadAccountsOptional() exists=%v err=%v", exists, err)
	}
	if a := store.Find("main"); a == nil || a.Cookies["remix_userid"] != "77" || a.Cookies["remix_userkey"] != "fresh-key" {
		t.Fatalf("active account should carry the fresh cookies, got %+v", store.Find("main"))
	}
}

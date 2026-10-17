package cli

import (
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

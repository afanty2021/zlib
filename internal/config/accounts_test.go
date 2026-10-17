package config

import (
	"os"
	"path/filepath"
	"testing"
)

func isolateAccountsHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func TestAccountsRoundTrip(t *testing.T) {
	isolateAccountsHome(t)

	remaining := 7
	store := AccountStore{
		Active: "main",
		Accounts: []Account{
			{Name: "main", Cookies: map[string]string{"remix_userid": "1"}, Domain: "https://zlib.bz", Mode: "eapi", Remaining: &remaining, CheckedAt: "2026-10-17T10:00:00+08:00"},
			{Name: "alt", Cookies: map[string]string{"remix_userid": "2"}, Domain: "https://librella.tw"},
		},
	}
	if err := SaveAccounts(store); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}

	info, err := os.Stat(AccountsPath())
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("accounts.json perm = %o, want 600", perm)
	}

	loaded, err := LoadAccounts()
	if err != nil {
		t.Fatalf("LoadAccounts() error = %v", err)
	}
	if loaded.Active != "main" || len(loaded.Accounts) != 2 {
		t.Fatalf("LoadAccounts() = %+v", loaded)
	}
	if got := loaded.Accounts[0].Remaining; got == nil || *got != 7 {
		t.Errorf("remaining = %v, want 7", got)
	}
}

func TestLoadAccountsOptionalMissingFile(t *testing.T) {
	isolateAccountsHome(t)

	store, exists, err := LoadAccountsOptional()
	if err != nil || exists || len(store.Accounts) != 0 {
		t.Fatalf("LoadAccountsOptional() = %+v, exists=%v, err=%v; want empty, false, nil", store, exists, err)
	}
}

func TestAccountStoreFindUpsertRemove(t *testing.T) {
	store := AccountStore{Accounts: []Account{{Name: "main"}}}

	store.Upsert(Account{Name: "alt"})
	if len(store.Accounts) != 2 || store.Find("alt") == nil {
		t.Fatalf("Upsert append: %+v", store.Accounts)
	}

	store.Upsert(Account{Name: "alt", Domain: "https://zlib.bz"})
	if len(store.Accounts) != 2 || store.Find("alt").Domain != "https://zlib.bz" {
		t.Fatalf("Upsert replace: %+v", store.Accounts)
	}

	if !store.Remove("main") || store.Find("main") != nil || len(store.Accounts) != 1 {
		t.Fatalf("Remove(main): %+v", store.Accounts)
	}
	if store.Remove("missing") {
		t.Error("Remove(missing) = true, want false")
	}
}

func TestAccountQuotaHelpers(t *testing.T) {
	var a Account

	if a.Exhausted() {
		t.Error("unknown quota must not count as exhausted")
	}

	a.DecrementRemaining()
	if a.Remaining != nil {
		t.Error("DecrementRemaining with unknown quota must be a no-op")
	}

	a.TouchQuota(2)
	a.DecrementRemaining()
	if a.Remaining == nil || *a.Remaining != 1 {
		t.Fatalf("remaining = %v, want 1", a.Remaining)
	}

	a.MarkExhausted()
	if !a.Exhausted() {
		t.Error("MarkExhausted must set exhausted")
	}
	if a.Remaining == nil || *a.Remaining != 0 {
		t.Errorf("remaining = %v, want 0", a.Remaining)
	}

	a.TouchQuota(-3)
	if a.Remaining == nil || *a.Remaining != 0 {
		t.Errorf("negative quota should clamp to 0, got %v", a.Remaining)
	}
}

func TestAccountSessionConversion(t *testing.T) {
	s := Session{Cookies: map[string]string{"remix_userid": "9"}, Domain: "https://zlib.bz", Mode: "eapi"}
	account := AccountFromSession("imported", s)

	rendered := account.Session()
	if account.Name != "imported" || rendered.Domain != s.Domain || rendered.Mode != s.Mode {
		t.Fatalf("AccountFromSession/Session round-trip = %+v", account)
	}
	if rendered.Cookies["remix_userid"] != "9" || len(rendered.Cookies) != len(s.Cookies) {
		t.Fatalf("Session() cookies = %v, want %v", rendered.Cookies, s.Cookies)
	}

	if _, err := os.Stat(filepath.Join("config", "zlib")); err == nil {
		t.Error("conversion must not touch the filesystem")
	}
}

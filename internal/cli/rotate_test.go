package cli

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/heartleo/zlib"
	"github.com/heartleo/zlib/internal/config"
)

func accountWithQuota(name string, remaining *int) config.Account {
	return config.Account{
		Name:      name,
		Cookies:   map[string]string{"remix_userid": name},
		Domain:    "https://zlib.bz",
		Mode:      "eapi",
		Remaining: remaining,
	}
}

func quotaPtr(n int) *int { return &n }

func TestRotationOrderActiveFirst(t *testing.T) {
	store := config.AccountStore{
		Active: "b",
		Accounts: []config.Account{
			accountWithQuota("a", nil),
			accountWithQuota("b", nil),
			accountWithQuota("c", nil),
		},
	}
	order := rotationOrder(&store)
	var names []string
	for _, account := range order {
		names = append(names, account.Name)
	}
	want := "b, a, c"
	if got := strings.Join(names, ", "); got != want {
		t.Fatalf("rotationOrder() = %q, want %q", got, want)
	}
}

func TestUsableAccountsSkipsExhausted(t *testing.T) {
	accounts := []*config.Account{
		accountWithQuotaPtr("a", quotaPtr(0)),
		accountWithQuotaPtr("b", quotaPtr(3)),
		accountWithQuotaPtr("c", nil),
	}
	// pointers into a slice the way rotationOrder produces them
	usable := usableAccounts(accounts)
	if len(usable) != 2 || usable[0].Name != "b" || usable[1].Name != "c" {
		names := make([]string, 0, len(usable))
		for _, a := range usable {
			names = append(names, a.Name)
		}
		t.Fatalf("usableAccounts() = %v, want [b c]", names)
	}
}

func accountWithQuotaPtr(name string, remaining *int) *config.Account {
	a := accountWithQuota(name, remaining)
	return &a
}

func TestRunRotatesToNextAccountOnDailyLimit(t *testing.T) {
	isolateConfigHome(t)

	store := config.AccountStore{
		Active: "a",
		Accounts: []config.Account{
			accountWithQuota("a", quotaPtr(1)),
			accountWithQuota("b", quotaPtr(5)),
		},
	}
	rotation := downloadRotation{store: store, exists: true}

	attempts := 0
	err := rotation.run(nil,
		func(account config.Account) (*zlib.Client, error) { return nil, nil },
		func(c *zlib.Client) error {
			attempts++
			if attempts == 1 {
				return fmt.Errorf("%w: Daily download limit reached", zlib.ErrDailyLimit)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}

	// Account a is exhausted, b is decremented and active, session mirrors b.
	if a := rotation.store.Find("a"); a == nil || !a.Exhausted() {
		t.Fatalf("account a should be marked exhausted, got %+v", rotation.store.Find("a"))
	}
	if b := rotation.store.Find("b"); b == nil || b.Remaining == nil || *b.Remaining != 4 {
		t.Fatalf("account b remaining = %v, want 4", rotation.store.Find("b").Remaining)
	}
	if rotation.store.Active != "b" {
		t.Fatalf("active = %q, want b", rotation.store.Active)
	}
	session, err := config.LoadSession()
	if err != nil || session.Cookies["remix_userid"] != "b" {
		t.Fatalf("session.json should mirror account b, got %+v (%v)", session, err)
	}

	// The persisted store matches the in-memory one.
	persisted, exists, err := config.LoadAccountsOptional()
	if err != nil || !exists || persisted.Active != "b" {
		t.Fatalf("persisted store = %+v exists=%v err=%v", persisted, exists, err)
	}
}

func TestRunReturnsOtherErrorsWithoutRotation(t *testing.T) {
	isolateConfigHome(t)

	rotation := downloadRotation{
		store: config.AccountStore{
			Active:   "a",
			Accounts: []config.Account{accountWithQuota("a", nil), accountWithQuota("b", nil)},
		},
		exists: true,
	}

	attempts := 0
	sentinel := errors.New("boom")
	err := rotation.run(nil,
		func(account config.Account) (*zlib.Client, error) { return nil, nil },
		func(c *zlib.Client) error {
			attempts++
			return sentinel
		})
	if !errors.Is(err, sentinel) {
		t.Fatalf("run() error = %v, want the original error", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (no rotation on unknown errors)", attempts)
	}
}

func TestRunRefreshesAfterDailyReset(t *testing.T) {
	isolateConfigHome(t)

	rotation := downloadRotation{
		store: config.AccountStore{
			Active:   "a",
			Accounts: []config.Account{accountWithQuota("a", quotaPtr(0)), accountWithQuota("b", quotaPtr(0))},
		},
		exists: true,
		limitsFetch: func(account config.Account) (zlib.DownloadLimit, error) {
			// The day rolled over: both accounts have quota again.
			return zlib.DownloadLimit{DailyAmount: 0, DailyAllowed: 10, DailyRemaining: 10}, nil
		},
	}

	attempts := 0
	err := rotation.run(nil,
		func(account config.Account) (*zlib.Client, error) { return nil, nil },
		func(c *zlib.Client) error {
			attempts++
			return nil
		})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (active account after refresh)", attempts)
	}
	if a := rotation.store.Find("a"); a == nil || a.Remaining == nil || *a.Remaining != 9 {
		t.Fatalf("account a remaining = %v, want 9 after refresh + download", rotation.store.Find("a").Remaining)
	}
}

func TestRunErrorsWhenEverythingExhausted(t *testing.T) {
	isolateConfigHome(t)

	rotation := downloadRotation{
		store: config.AccountStore{
			Active:   "a",
			Accounts: []config.Account{accountWithQuota("a", quotaPtr(0)), accountWithQuota("b", quotaPtr(0))},
		},
		exists: true,
		limitsFetch: func(account config.Account) (zlib.DownloadLimit, error) {
			return zlib.DownloadLimit{DailyRemaining: 0}, nil
		},
	}

	err := rotation.run(nil,
		func(account config.Account) (*zlib.Client, error) { return nil, nil },
		func(c *zlib.Client) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "every account") {
		t.Fatalf("run() error = %v, want all-accounts-exhausted error", err)
	}
}

func TestRunSingleSessionUsesAmbientClient(t *testing.T) {
	isolateConfigHome(t)

	rotation := downloadRotation{exists: false}
	attempts := 0
	err := rotation.run(nil, func(account config.Account) (*zlib.Client, error) { return nil, nil }, func(c *zlib.Client) error {
		attempts++
		return nil
	})
	if err == nil || attempts != 0 {
		t.Fatalf("ambient nil client must error, got err=%v attempts=%d", err, attempts)
	}

	err = rotation.run(zlib.NewClient(), func(account config.Account) (*zlib.Client, error) { return nil, nil }, func(c *zlib.Client) error {
		attempts++
		return nil
	})
	if err != nil || attempts != 1 {
		t.Fatalf("ambient client should be attempted once, got err=%v attempts=%d", err, attempts)
	}
}

func TestRecordAccountQuotaUpdatesActiveAccount(t *testing.T) {
	isolateConfigHome(t)

	store := config.AccountStore{
		Active:   "a",
		Accounts: []config.Account{accountWithQuota("a", quotaPtr(0))},
	}
	if err := config.SaveAccounts(store); err != nil {
		t.Fatalf("SaveAccounts() error = %v", err)
	}

	recordAccountQuota(zlib.DownloadLimit{DailyRemaining: 7})
	reloaded, exists, err := config.LoadAccountsOptional()
	if err != nil || !exists {
		t.Fatalf("LoadAccountsOptional() exists=%v err=%v", exists, err)
	}
	if a := reloaded.Find("a"); a == nil || a.Remaining == nil || *a.Remaining != 7 {
		t.Fatalf("account a remaining = %v, want 7", reloaded.Find("a").Remaining)
	}
}

func TestAccountFromRemixCredentialsDomainFallback(t *testing.T) {
	t.Run("flag wins", func(t *testing.T) {
		t.Setenv(zlib.EnvDomain, "https://z-lib.gl")
		account, err := accountFromRemixCredentials("main", "1", "k", "https://zlib.bz", false)
		if err != nil || account.Domain != "https://zlib.bz" {
			t.Fatalf("account = %+v err = %v", account, err)
		}
	})
	t.Run("env fallback", func(t *testing.T) {
		t.Setenv(zlib.EnvDomain, "https://z-lib.gl")
		account, err := accountFromRemixCredentials("main", "1", "k", "", false)
		if err != nil || account.Domain != "https://z-lib.gl" {
			t.Fatalf("account = %+v err = %v", account, err)
		}
	})
	t.Run("eapi default", func(t *testing.T) {
		account, err := accountFromRemixCredentials("main", "1", "k", "", true)
		if err != nil || account.Domain != zlib.DefaultEAPIDomain || account.Mode != "eapi" {
			t.Fatalf("account = %+v err = %v", account, err)
		}
	})
	t.Run("html requires domain", func(t *testing.T) {
		if _, err := accountFromRemixCredentials("main", "1", "k", "", false); err == nil {
			t.Fatal("expected domain-required error")
		}
	})
	t.Run("rejects disallowed domain", func(t *testing.T) {
		if _, err := accountFromRemixCredentials("main", "1", "k", "https://evil.example", true); err == nil {
			t.Fatal("expected allowlist error")
		}
	})
	t.Run("requires both halves", func(t *testing.T) {
		if _, err := accountFromRemixCredentials("main", "1", "", "https://zlib.bz", true); err == nil {
			t.Fatal("expected missing --user-key error")
		}
	})
}

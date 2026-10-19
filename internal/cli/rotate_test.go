package cli

import (
	"errors"
	"fmt"
	"net/http"
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
		account, err := accountFromRemixCredentials("main", "1", "k", "https://zlib.bz")
		if err != nil || account.Domain != "https://zlib.bz" {
			t.Fatalf("account = %+v err = %v", account, err)
		}
	})
	t.Run("env fallback", func(t *testing.T) {
		t.Setenv(zlib.EnvDomain, "https://z-lib.gl")
		account, err := accountFromRemixCredentials("main", "1", "k", "")
		if err != nil || account.Domain != "https://z-lib.gl" {
			t.Fatalf("account = %+v err = %v", account, err)
		}
	})
	t.Run("eapi default", func(t *testing.T) {
		account, err := accountFromRemixCredentials("main", "1", "k", "")
		if err != nil || account.Domain != zlib.DefaultEAPIDomain || account.Mode != "eapi" {
			t.Fatalf("account = %+v err = %v", account, err)
		}
	})
	t.Run("rejects disallowed domain", func(t *testing.T) {
		if _, err := accountFromRemixCredentials("main", "1", "k", "https://evil.example"); err == nil {
			t.Fatal("expected allowlist error")
		}
	})
	t.Run("requires both halves", func(t *testing.T) {
		if _, err := accountFromRemixCredentials("main", "1", "", "https://zlib.bz"); err == nil {
			t.Fatal("expected missing --user-key error")
		}
	})
}

func TestRunRecoversStaleExhaustedAccountMidLoop(t *testing.T) {
	isolateConfigHome(t)

	// a is cached exhausted from last night but its quota actually reset;
	// b burns out mid-download. The post-loop refresh must revive a and the
	// retry pass must succeed on it instead of reporting all-exhausted.
	rotation := downloadRotation{
		store: config.AccountStore{
			Active:   "b",
			Accounts: []config.Account{accountWithQuota("a", quotaPtr(0)), accountWithQuota("b", nil)},
		},
		exists: true,
		limitsFetch: func(account config.Account) (zlib.DownloadLimit, error) {
			if account.Name == "a" {
				return zlib.DownloadLimit{DailyRemaining: 10}, nil
			}
			return zlib.DownloadLimit{DailyRemaining: 0}, nil
		},
	}

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
	if err != nil || attempts != 2 {
		t.Fatalf("run() err=%v attempts=%d, want 2 attempts and success", err, attempts)
	}
	if a := rotation.store.Find("a"); a == nil || a.Remaining == nil || *a.Remaining != 9 {
		t.Fatalf("account a remaining = %v, want 9 after refresh + download", rotation.store.Find("a").Remaining)
	}
	if rotation.store.Active != "a" {
		t.Fatalf("active = %q, want a after the successful retry", rotation.store.Active)
	}
}

func TestConfirmHTMLNoDownloadURLRotatesOnConfirmedZero(t *testing.T) {
	isolateConfigHome(t)

	rotation := downloadRotation{
		store: config.AccountStore{
			Active:   "a",
			Accounts: []config.Account{accountWithQuota("a", nil), accountWithQuota("b", nil)},
		},
		exists: true,
		limitsFetch: func(account config.Account) (zlib.DownloadLimit, error) {
			return zlib.DownloadLimit{DailyRemaining: 0}, nil
		},
	}

	attempts := 0
	err := rotation.run(nil,
		func(account config.Account) (*zlib.Client, error) { return nil, nil },
		func(c *zlib.Client) error {
			attempts++
			if attempts == 1 {
				// The HTML shape: quota exhaustion shows up as the book page
				// offering no download button at all.
				return fmt.Errorf("%w: 12345", zlib.ErrNoDownloadURL)
			}
			return nil
		})
	if err != nil || attempts != 2 {
		t.Fatalf("run() err=%v attempts=%d, want rotation to b and success", err, attempts)
	}
	if a := rotation.store.Find("a"); a == nil || !a.Exhausted() {
		t.Fatal("account a should be marked exhausted after confirmation")
	}
}

func TestConfirmHTMLKeepsErrorWhenQuotaRemains(t *testing.T) {
	isolateConfigHome(t)

	rotation := downloadRotation{
		store: config.AccountStore{
			Active:   "a",
			Accounts: []config.Account{accountWithQuota("a", nil), accountWithQuota("b", nil)},
		},
		exists: true,
		limitsFetch: func(account config.Account) (zlib.DownloadLimit, error) {
			return zlib.DownloadLimit{DailyRemaining: 3}, nil
		},
	}

	attempts := 0
	downloadErr := fmt.Errorf("%w: 12345", zlib.ErrNoDownloadURL)
	err := rotation.run(nil,
		func(account config.Account) (*zlib.Client, error) { return nil, nil },
		func(c *zlib.Client) error {
			attempts++
			return downloadErr
		})
	if !errors.Is(err, zlib.ErrNoDownloadURL) {
		t.Fatalf("run() err = %v, want the original no-download-URL error", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (no rotation with quota left)", attempts)
	}
	if a := rotation.store.Find("a"); a == nil || a.Remaining == nil || *a.Remaining != 3 {
		t.Fatalf("confirmed quota should be cached, got %v", rotation.store.Find("a").Remaining)
	}
}

func TestConfirmSkipsQuotaLookupForTransientStatuses(t *testing.T) {
	isolateConfigHome(t)

	lookups := 0
	rotation := downloadRotation{
		store: config.AccountStore{
			Active:   "a",
			Accounts: []config.Account{accountWithQuota("a", nil), accountWithQuota("b", nil)},
		},
		exists: true,
		limitsFetch: func(account config.Account) (zlib.DownloadLimit, error) {
			lookups++
			return zlib.DownloadLimit{DailyRemaining: 0}, nil
		},
	}

	gatewayErr := &zlib.DownloadHTTPError{StatusCode: http.StatusServiceUnavailable}
	err := rotation.run(nil,
		func(account config.Account) (*zlib.Client, error) { return nil, nil },
		func(c *zlib.Client) error { return gatewayErr })
	if !errors.Is(err, zlib.ErrDownloadFailed) {
		t.Fatalf("run() err = %v, want the original gateway error", err)
	}
	if lookups != 0 {
		t.Fatalf("quota lookups = %d, want 0 for a 503 failure", lookups)
	}
}

func TestRunDistinguishesLookupFailureFromExhaustion(t *testing.T) {
	isolateConfigHome(t)

	rotation := downloadRotation{
		store: config.AccountStore{
			Active:   "a",
			Accounts: []config.Account{accountWithQuota("a", quotaPtr(0)), accountWithQuota("b", quotaPtr(0))},
		},
		exists: true,
		limitsFetch: func(account config.Account) (zlib.DownloadLimit, error) {
			return zlib.DownloadLimit{}, errors.New("connection refused")
		},
	}

	attempts := 0
	err := rotation.run(nil,
		func(account config.Account) (*zlib.Client, error) { return nil, nil },
		func(c *zlib.Client) error {
			attempts++
			return nil
		})
	if err == nil || !strings.Contains(err.Error(), "re-check failed") {
		t.Fatalf("run() err = %v, want the quota-lookup-failure wording", err)
	}
	if attempts != 0 {
		t.Fatalf("attempts = %d, want 0 (nothing was confirmed usable)", attempts)
	}
}

func TestConfirmEAPINoFileLinkRotatesOnConfirmedZero(t *testing.T) {
	isolateConfigHome(t)

	rotation := downloadRotation{
		store: config.AccountStore{
			Active:   "a",
			Accounts: []config.Account{accountWithQuota("a", nil), accountWithQuota("b", nil)},
		},
		exists: true,
		limitsFetch: func(account config.Account) (zlib.DownloadLimit, error) {
			return zlib.DownloadLimit{DailyRemaining: 0}, nil
		},
	}

	attempts := 0
	err := rotation.run(nil,
		func(account config.Account) (*zlib.Client, error) { return nil, nil },
		func(c *zlib.Client) error {
			attempts++
			if attempts == 1 {
				// The EAPI shape measured live: success=1 with no download
				// link and no error string when the daily quota is spent.
				return fmt.Errorf("%w: %w", zlib.ErrDownloadFailed, zlib.ErrNoFileLink)
			}
			return nil
		})
	if err != nil || attempts != 2 {
		t.Fatalf("run() err=%v attempts=%d, want rotation to b and success", err, attempts)
	}
	if a := rotation.store.Find("a"); a == nil || !a.Exhausted() {
		t.Fatal("account a should be marked exhausted after confirmation")
	}
}

func TestConfirmEAPINoFileLinkKeepsErrorWhenQuotaRemains(t *testing.T) {
	isolateConfigHome(t)

	rotation := downloadRotation{
		store: config.AccountStore{
			Active:   "a",
			Accounts: []config.Account{accountWithQuota("a", nil), accountWithQuota("b", nil)},
		},
		exists: true,
		limitsFetch: func(account config.Account) (zlib.DownloadLimit, error) {
			return zlib.DownloadLimit{DailyRemaining: 4}, nil
		},
	}

	attempts := 0
	err := rotation.run(nil,
		func(account config.Account) (*zlib.Client, error) { return nil, nil },
		func(c *zlib.Client) error {
			attempts++
			return fmt.Errorf("%w: %w", zlib.ErrDownloadFailed, zlib.ErrNoFileLink)
		})
	if err == nil || attempts != 1 {
		t.Fatalf("run() err=%v attempts=%d, want the no-link error kept, no rotation", err, attempts)
	}
	if a := rotation.store.Find("a"); a == nil || a.Exhausted() {
		t.Fatal("account a must not be marked exhausted while quota remains")
	}
}

func TestCancelledDownloadKeepsAccountUsable(t *testing.T) {
	isolateConfigHome(t)

	rotation := downloadRotation{
		store: config.AccountStore{
			Active:   "a",
			Accounts: []config.Account{accountWithQuota("a", ptrInt(3)), accountWithQuota("b", ptrInt(9))},
		},
		exists: true,
	}

	attempts := 0
	err := rotation.run(nil,
		func(account config.Account) (*zlib.Client, error) { return nil, nil },
		func(c *zlib.Client) error {
			attempts++
			return errDownloadCancelled
		})
	if err != nil || attempts != 1 {
		t.Fatalf("run() err=%v attempts=%d, want a clean stop after one attempt", err, attempts)
	}
	if a := rotation.store.Find("a"); a == nil || a.Remaining == nil || *a.Remaining != 3 {
		t.Fatalf("account a remaining = %v, want the 3 it started with", a.Remaining)
	}
	if rotation.store.Active != "a" {
		t.Fatalf("active = %q, want a; a cancel must not switch accounts", rotation.store.Active)
	}
}

func ptrInt(v int) *int { return &v }

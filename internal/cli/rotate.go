package cli

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/heartleo/zlib"
	"github.com/heartleo/zlib/internal/config"
)

// downloadRotation drives a download across the configured accounts: it tries
// the active account first and, when the server rejects the download with the
// daily-limit error, marks that account exhausted and retries with the next
// account that may still have quota. With no accounts.json it degrades to the
// single-session behaviour that predates multi-account support.
type downloadRotation struct {
	store config.AccountStore
	// exists is false when accounts.json is absent: single-session mode.
	exists bool
	// limitsFetch overrides quota lookups in tests; nil queries the server.
	limitsFetch func(config.Account) (zlib.DownloadLimit, error)
}

// loadDownloadRotation reads accounts.json. A store that cannot be parsed,
// or that has no accounts left, disables rotation rather than breaking the
// download — with a warning, because silently losing rotation is confusing.
func loadDownloadRotation() downloadRotation {
	store, exists, err := config.LoadAccountsOptional()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %s could not be read (%v); falling back to the single-session login.\n", config.AccountsPath(), err)
		return downloadRotation{}
	}
	if !exists || len(store.Accounts) == 0 {
		return downloadRotation{}
	}
	return downloadRotation{store: store, exists: true}
}

// rotationOrder lists every account a download may try, in order: the active
// account first, then the rest as stored. Exhausted accounts stay in the list
// so the refresh path can see them; pickers skip them.
func rotationOrder(store *config.AccountStore) []*config.Account {
	var order []*config.Account
	if a := store.Find(store.Active); a != nil {
		order = append(order, a)
	}
	for i := range store.Accounts {
		if store.Accounts[i].Name == store.Active {
			continue
		}
		order = append(order, &store.Accounts[i])
	}
	return order
}

// usableAccounts filters out accounts already known to be out of quota.
func usableAccounts(accounts []*config.Account) []*config.Account {
	var usable []*config.Account
	for _, account := range accounts {
		if !account.Exhausted() {
			usable = append(usable, account)
		}
	}
	return usable
}

// errCandidatesExhausted is the internal marker that every usable candidate
// hit its daily limit (or none were usable); run() turns it into one quota
// refresh pass before reporting.
var errCandidatesExhausted = errors.New("all candidate accounts exhausted")

// run executes attempt per candidate account. build makes the client for an
// account; ambient is the client newClient() already built and is used only in
// single-session mode. Quota bookkeeping and the active-account mirror are
// persisted as attempts happen, so an interrupted run still leaves an accurate
// store behind.
//
// When every usable candidate hits the daily limit — including the case where
// an account's cached quota went stale overnight — run re-checks quotas with
// the server once and retries whatever became usable, bounded to that single
// extra pass.
func (r *downloadRotation) run(
	ambient *zlib.Client,
	build func(config.Account) (*zlib.Client, error),
	attempt func(*zlib.Client) error,
) error {
	if !r.exists {
		if ambient == nil {
			return errors.New("no account store and no client to download with")
		}
		return attempt(ambient)
	}

	if err := r.attemptLoop(build, attempt); err == nil || !errors.Is(err, errCandidatesExhausted) {
		return err
	}

	usable, lookupErr := r.refreshAll()
	if !usable {
		if lookupErr != nil {
			return fmt.Errorf("every account reached its daily limit, and the quota re-check failed too (%v); try again later", lookupErr)
		}
		return r.exhaustedError()
	}

	err := r.attemptLoop(build, attempt)
	if err == nil || !errors.Is(err, errCandidatesExhausted) {
		return err
	}
	return r.exhaustedError()
}

// attemptLoop tries every usable account once, active first. It returns nil
// once an attempt succeeds, the aborting error for anything that is not the
// daily limit, and errCandidatesExhausted when the whole loop was spent
// hitting limits.
func (r *downloadRotation) attemptLoop(
	build func(config.Account) (*zlib.Client, error),
	attempt func(*zlib.Client) error,
) error {
	candidates := usableAccounts(rotationOrder(&r.store))
	if len(candidates) == 0 {
		return errCandidatesExhausted
	}
	for _, account := range candidates {
		client, err := build(*account)
		if err != nil {
			return err
		}
		err = attempt(client)
		switch {
		case err == nil:
			account.DecrementRemaining()
			if r.store.Active != account.Name {
				r.store.Active = account.Name
				if sessErr := config.SaveSession(account.Session()); sessErr != nil {
					return fmt.Errorf("failed to save session: %w", sessErr)
				}
			}
			if err := r.saveStore(); err != nil {
				return err
			}
			return nil
		case errors.Is(err, zlib.ErrDailyLimit) || r.confirmDailyLimit(account, err):
			fmt.Printf("%s Account %s reached its daily limit; trying the next account.\n",
				colorCyanFn(symbolInfo), account.Name)
			account.MarkExhausted()
			if err := r.saveStore(); err != nil {
				return err
			}
			continue
		default:
			return err
		}
	}
	return errCandidatesExhausted
}

// saveStore writes the account store back after in-place quota edits.
func (r *downloadRotation) saveStore() error {
	return config.SaveAccounts(r.store)
}

// confirmDailyLimit re-checks the account's quota when a download failed in
// a way the server uses for both "quota gone" and unrelated problems: HTML
// pages drop the download link or answer 403, and the EAPI answers success
// with no file link. Only a zero remaining count turns the failure into a
// rotation trigger; otherwise the original error stands.
func (r *downloadRotation) confirmDailyLimit(account *config.Account, err error) bool {
	var httpErr *zlib.DownloadHTTPError
	confirmable := errors.Is(err, zlib.ErrNoDownloadURL) ||
		errors.Is(err, zlib.ErrNoFileLink) ||
		(errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusForbidden)
	if !confirmable {
		return false
	}
	limits, limitsErr := r.fetchLimits(*account)
	if limitsErr != nil {
		return false
	}
	account.TouchQuota(limits.DailyRemaining)
	_ = r.saveStore()
	return limits.DailyRemaining <= 0
}

// fetchLimits looks up an account's current quota, through the test hook when
// one is installed.
func (r *downloadRotation) fetchLimits(account config.Account) (zlib.DownloadLimit, error) {
	if r.limitsFetch != nil {
		return r.limitsFetch(account)
	}
	client, err := clientFromSession(account.Session())
	if err != nil {
		return zlib.DownloadLimit{}, err
	}
	return client.GetLimits()
}

// refreshAll re-queries every account's quota. usable reports whether any
// account still has downloads left; lookupErr is set when every lookup
// failed, so callers can tell "confirmed all-exhausted" apart from "the
// network broke mid-check". This is the daily-reset recovery path: the EAPI
// does not expose the reset time, so a fresh lookup is the only way to notice
// the quota came back.
func (r *downloadRotation) refreshAll() (usable bool, lookupErr error) {
	fmt.Printf("%s Attempts hit the daily limit everywhere; re-checking quotas with the server.\n", colorCyanFn(symbolInfo))
	failures := 0
	var lastErr error
	for _, account := range rotationOrder(&r.store) {
		limits, err := r.fetchLimits(*account)
		if err != nil {
			failures++
			lastErr = err
			continue
		}
		account.TouchQuota(limits.DailyRemaining)
		if !account.Exhausted() {
			usable = true
		}
	}
	_ = r.saveStore()
	if !usable && failures == len(r.store.Accounts) && failures > 0 {
		lookupErr = lastErr
	}
	return usable, lookupErr
}

// exhaustedError reports the all-accounts-done state with each account's last
// known remaining, so the user can see when to expect a reset.
func (r *downloadRotation) exhaustedError() error {
	parts := make([]string, 0, len(r.store.Accounts))
	for _, account := range r.store.Accounts {
		remaining := "unknown"
		if account.Remaining != nil {
			remaining = strconv.Itoa(*account.Remaining)
		}
		parts = append(parts, fmt.Sprintf("%s (%s left)", account.Name, remaining))
	}
	return fmt.Errorf("daily download limit reached on every account: %s", strings.Join(parts, ", "))
}

// recordAccountQuota caches a freshly observed quota against the active
// account when accounts are configured, so rotation decisions (and
// `accounts list`) stay current. A failure to persist is not worth failing
// the profile command over.
func recordAccountQuota(limits zlib.DownloadLimit) {
	store, exists, err := config.LoadAccountsOptional()
	if err != nil || !exists {
		return
	}
	account := store.Find(store.Active)
	if account == nil {
		return
	}
	account.TouchQuota(limits.DailyRemaining)
	_ = config.SaveAccounts(store)
}

// runDownloadWithRotation wraps runDownload with multi-account rotation.
// model builds the download model for a given client; every download entry
// point (download, search Enter, history) goes through here.
func runDownloadWithRotation(model func(*zlib.Client) downloadModel, sendToKindle bool, cmd *cobra.Command) error {
	rotation := loadDownloadRotation()
	if !rotation.exists {
		return runDownload(model(newClient()), sendToKindle, cmd)
	}
	return rotation.run(nil, func(account config.Account) (*zlib.Client, error) {
		return clientFromSession(account.Session())
	}, func(c *zlib.Client) error {
		return runDownload(model(c), sendToKindle, cmd)
	})
}

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Account is one named Z-Library credential set plus the last known daily
// download quota observed for it.
type Account struct {
	Name    string            `json:"name"`
	Cookies map[string]string `json:"cookies"`
	Domain  string            `json:"domain"`
	Mode    string            `json:"mode,omitempty"`
	// Remaining is the last observed daily-download remaining. Nil means
	// unknown; zero means exhausted for the rest of the day.
	Remaining *int `json:"remaining,omitempty"`
	// CheckedAt is when Remaining was last observed, RFC3339.
	CheckedAt string `json:"checked_at,omitempty"`
}

// AccountStore holds the configured accounts and the name of the currently
// active one. The active account is mirrored into session.json, which stays
// the single session every command reads; switching accounts rewrites it.
type AccountStore struct {
	Active   string    `json:"active"`
	Accounts []Account `json:"accounts"`
}

func AccountsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "zlib", "accounts.json")
}

func LoadAccounts() (AccountStore, error) {
	data, err := os.ReadFile(AccountsPath())
	if err != nil {
		return AccountStore{}, err
	}
	var s AccountStore
	return s, json.Unmarshal(data, &s)
}

// LoadAccountsOptional returns the store and whether accounts.json exists, so
// callers can fall back to single-session behaviour when it does not.
func LoadAccountsOptional() (AccountStore, bool, error) {
	store, err := LoadAccounts()
	if os.IsNotExist(err) {
		return AccountStore{}, false, nil
	}
	if err != nil {
		return AccountStore{}, false, err
	}
	return store, true, nil
}

func SaveAccounts(s AccountStore) error {
	return saveJSONAtomic(AccountsPath(), s)
}

// Find returns the account with the given name, or nil.
func (s *AccountStore) Find(name string) *Account {
	for i := range s.Accounts {
		if s.Accounts[i].Name == name {
			return &s.Accounts[i]
		}
	}
	return nil
}

// Upsert adds an account, or replaces the existing one with the same name.
func (s *AccountStore) Upsert(a Account) {
	if existing := s.Find(a.Name); existing != nil {
		*existing = a
		return
	}
	s.Accounts = append(s.Accounts, a)
}

// Remove deletes an account by name and reports whether it existed.
func (s *AccountStore) Remove(name string) bool {
	for i := range s.Accounts {
		if s.Accounts[i].Name == name {
			s.Accounts = append(s.Accounts[:i], s.Accounts[i+1:]...)
			return true
		}
	}
	return false
}

// Session renders the account as the session.json payload.
func (a Account) Session() Session {
	return Session{Cookies: a.Cookies, Domain: a.Domain, Mode: a.Mode}
}

// Exhausted reports whether the cached quota says the account is done for
// today. Unknown (nil) counts as not exhausted; only a measured zero does.
func (a *Account) Exhausted() bool {
	return a.Remaining != nil && *a.Remaining <= 0
}

// TouchQuota records a freshly observed remaining count.
func (a *Account) TouchQuota(remaining int) {
	value := remaining
	if value < 0 {
		value = 0
	}
	a.Remaining = &value
	a.CheckedAt = time.Now().Format(time.RFC3339)
}

// MarkExhausted records that the account hit its daily limit.
func (a *Account) MarkExhausted() {
	a.TouchQuota(0)
}

// DecrementRemaining steps the cached quota down after a successful download.
// It is a no-op while the quota is unknown.
func (a *Account) DecrementRemaining() {
	if a.Remaining == nil || *a.Remaining <= 0 {
		return
	}
	value := *a.Remaining - 1
	a.Remaining = &value
}

// AccountFromSession promotes a session.json payload to a named account.
func AccountFromSession(name string, s Session) Account {
	return Account{Name: name, Cookies: s.Cookies, Domain: s.Domain, Mode: s.Mode}
}

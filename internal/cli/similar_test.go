package cli

import (
	"strings"
	"testing"

	"github.com/heartleo/zlib/internal/config"
)

// The gate must fire before any network attempt: an HTML session cannot
// address these commands at all, and the failure must name the fix. The
// session domain is a real allowlisted mirror because client construction
// validates it before the RunE body runs.
func TestSimilarAndSeriesRejectHTMLModeSession(t *testing.T) {
	isolateConfigHome(t)
	if err := config.SaveSession(config.Session{
		Cookies: map[string]string{"remix_userid": "1", "remix_userkey": "seed-key"},
		Domain:  "https://zlib.bz",
		Mode:    "html",
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	for _, command := range []string{"similar", "series"} {
		_, err := executeRootForTest(t, command, "13955378:a49630")
		if err == nil || !strings.Contains(err.Error(), "requires an EAPI session") {
			t.Fatalf("%s on HTML session: err = %v, want EAPI gate error", command, err)
		}
	}
}

// The EAPI happy path cannot be driven at command level: client construction
// validates the session domain against the mirror allowlist (HTTPS, no port),
// so no httptest origin can ever reach the command body. The endpoint mapping
// is covered in the library package; the writer routing is structural
// (printJSONTo(cmd.OutOrStdout())) per the accounts --json precedent.

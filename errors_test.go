package zlib

import (
	"errors"
	"testing"
)

func TestDownloadHTTPErrorClassifiesLikeItsStringForm(t *testing.T) {
	err := &DownloadHTTPError{StatusCode: 403}
	if got, want := err.Error(), "zlibrary: download failed: HTTP 403"; got != want {
		t.Fatalf("Error() = %q, want %q (message parity with the old fmt.Errorf wrap)", got, want)
	}
	if !errors.Is(err, ErrDownloadFailed) {
		t.Fatal("DownloadHTTPError must unwrap to ErrDownloadFailed")
	}
	var typed *DownloadHTTPError
	if !errors.As(error(err), &typed) || typed.StatusCode != 403 {
		t.Fatal("DownloadHTTPError must be discoverable with errors.As for status gating")
	}
}

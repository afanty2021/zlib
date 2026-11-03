package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/heartleo/zlib/internal/config"
	"github.com/heartleo/zlib/internal/zlib"
)

// replacePicker swaps the rotation seam's directory picker for fakePick and
// restores it when the test ends.
func replacePicker(t *testing.T, fakePick func(cmd *cobra.Command, dir string) (string, bool, error)) {
	t.Helper()
	old := pickDownloadDirectory
	pickDownloadDirectory = fakePick
	t.Cleanup(func() { pickDownloadDirectory = old })
}

// saveRotationStore persists a one-account store so the seam runs in
// rotation mode. The account's zlib.bz session needs no working server: the
// test models download from a direct local URL.
func saveRotationStore(t *testing.T) {
	t.Helper()
	if err := config.SaveAccounts(config.AccountStore{
		Active:   "a",
		Accounts: []config.Account{accountWithQuota("a", ptrInt(3))},
	}); err != nil {
		t.Fatal(err)
	}
}

// The directory the picker answers once up front must reach the download on
// every attempt: the command-layer closures captured the pre-picker dir, so
// the seam has to re-apply the picked dir to the model (and pin --dir so
// runDownload never re-opens the picker mid-rotation).
func TestRotationDownloadAppliesPickedDirectory(t *testing.T) {
	isolateConfigHome(t)
	saveRotationStore(t)
	body := []byte("book")
	server := newDownloadTestServer(t, body)

	flagDir, pickedDir := t.TempDir(), t.TempDir()
	replacePicker(t, func(cmd *cobra.Command, dir string) (string, bool, error) {
		if dir != flagDir {
			t.Errorf("picker received dir %q, want the pre-resolution dir %q", dir, flagDir)
		}
		return pickedDir, true, nil
	})

	cmd := downloadTestCommand(t, false)
	var runErr error
	captureOutput(t, func() {
		runErr = runDownloadWithRotation(func(c *zlib.Client) downloadModel {
			return newDirectDownloadModel("Dune", server.URL+"/files/dune", flagDir, c)
		}, flagDir, false, cmd)
	})
	if runErr != nil {
		t.Fatalf("runDownloadWithRotation error = %v", runErr)
	}
	got, err := os.ReadFile(filepath.Join(pickedDir, "Dune.epub"))
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("the file must land in the picked dir: %v", err)
	}
	if pinned, _ := cmd.Flags().GetString("dir"); pinned != pickedDir {
		t.Fatalf("--dir pinned to %q, want the picked dir %q", pinned, pickedDir)
	}
}

// --json keeps stdout to the result document, so the rotation seam must not
// open the interactive picker before the first attempt either.
func TestRotationDownloadJSONSkipsDirectoryPicker(t *testing.T) {
	isolateConfigHome(t)
	saveRotationStore(t)
	server := newDownloadTestServer(t, []byte("book"))

	pickerCalled := false
	replacePicker(t, func(cmd *cobra.Command, dir string) (string, bool, error) {
		pickerCalled = true
		return dir, true, nil
	})

	// The model keeps its own dir: without a picker answer there is nothing
	// to re-apply, and the model dir (not the empty input dir) must win.
	modelDir := t.TempDir()
	var runErr error
	stdout, _ := captureOutput(t, func() {
		runErr = runDownloadWithRotation(func(c *zlib.Client) downloadModel {
			return newDirectDownloadModel("Dune", server.URL+"/files/dune", modelDir, c)
		}, "", false, downloadTestCommand(t, true))
	})
	if runErr != nil {
		t.Fatalf("runDownloadWithRotation error = %v", runErr)
	}
	if pickerCalled {
		t.Fatal("the directory picker must not open under --json")
	}
	var result downloadResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, stdout)
	}
	if _, err := os.Stat(filepath.Join(modelDir, "Dune.epub")); err != nil {
		t.Fatalf("the file must land in the model's dir: %v", err)
	}
}

// Refusing the picker cancels before any account is tried, with the same
// errCancelled the single-session path reports (exit code 130).
func TestRotationDownloadRefusalCancels(t *testing.T) {
	isolateConfigHome(t)
	saveRotationStore(t)
	replacePicker(t, func(cmd *cobra.Command, dir string) (string, bool, error) {
		return "", false, nil
	})

	err := runDownloadWithRotation(func(c *zlib.Client) downloadModel {
		t.Error("no download must run after a picker refusal")
		return downloadModel{}
	}, ".", false, downloadTestCommand(t, false))
	if !errors.Is(err, errCancelled) {
		t.Fatalf("err = %v, want errCancelled", err)
	}
}

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// saveJSONAtomic writes v to path via a temp file + rename, so a crash or
// concurrent reader never sees a truncated store. These files hold session
// credentials: an in-place os.WriteFile that dies mid-write would destroy the
// only copy.
func saveJSONAtomic(path string, v any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	// A no-op after a successful rename; cleans up on any early return.
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp is 0600 on Unix but more permissive elsewhere; make the
	// destination's permissions explicit.
	if err := os.Chmod(tmp.Name(), 0600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

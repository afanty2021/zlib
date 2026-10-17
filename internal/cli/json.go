package cli

import (
	"encoding/json"
	"io"
	"os"
)

// printJSON writes v to stdout as a single JSON document with a trailing
// newline and no decoration, so `--json` output is directly consumable by
// jq, scripts, and the Claude Code plugin.
func printJSON(v any) error {
	return printJSONTo(os.Stdout, v)
}

// printJSONTo is printJSON aimed at an explicit writer, so commands routed
// through cobra keep testability instead of writing straight to os.Stdout.
func printJSONTo(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	return enc.Encode(v)
}

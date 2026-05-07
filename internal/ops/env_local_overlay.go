package ops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// envLocalHeader is the fixed comment header ZCP writes into a freshly-
// created `.env.local`. Stable string — pinned by tests so future
// edits stay deliberate.
const envLocalHeader = `# Created by ZCP. Edit freely — ZCP merges these values into .env at
# every generate-dotenv but will not overwrite this file.
# Add ".env.local" to .gitignore if not already there.
`

// ErrEnvLocalAlreadyExists is returned by EnsureEnvLocal when the file
// is already present. ZCP MUST NOT overwrite a user-authored
// .env.local — once created, the file is the user's no-touch zone.
var ErrEnvLocalAlreadyExists = errors.New(".env.local already exists; ZCP never overwrites it")

// EnsureEnvLocal writes a `.env.local` into cwd if and only if the
// file is absent. Used by recipe-local bootstrap (Theme 1) to seed
// local-mode flags from a recipe's dev setup, and by brownfield-adopt
// (Theme 3) to seed classified local-only entries from the user's
// existing `.env`.
//
// Returns ErrEnvLocalAlreadyExists when the file is already there;
// the caller decides whether that's expected or an error.
//
// `seed` may be nil — the file then contains only the header.
//
// Writers other than this function MUST NOT touch `.env.local` —
// pinned by docs/spec-env-handling.md §7.1 (single-writer contract).
func EnsureEnvLocal(cwd string, seed map[string]string) error {
	path := filepath.Join(cwd, ".env.local")
	if _, err := os.Stat(path); err == nil {
		return ErrEnvLocalAlreadyExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat .env.local: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(envLocalHeader)
	if len(seed) > 0 {
		sb.WriteByte('\n')
		keys := make([]string, 0, len(seed))
		for k := range seed {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(k)
			sb.WriteByte('=')
			sb.WriteString(seed[k])
			sb.WriteByte('\n')
		}
	}

	// Use atomic write so a crashed creation doesn't leave a partial
	// .env.local that the next call would refuse to overwrite.
	if err := atomicWriteFile(path, []byte(sb.String()), 0600); err != nil {
		return fmt.Errorf("write .env.local: %w", err)
	}
	return nil
}

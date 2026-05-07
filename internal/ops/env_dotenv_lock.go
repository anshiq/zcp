//go:build unix

package ops

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// acquireDotenvLock takes an exclusive advisory lock per (workingDir,
// setup) so concurrent regens of the same target serialize. The lock
// is per-setup: regen of `prod` does not block regen of `worker` in
// a multi-setup project. See docs/spec-env-handling.md §6.4.
//
// Caller MUST defer the returned release function on success. Lock
// file lives at <workingDir>/.zcp/state/locks/dotenv-<setup>.lock
// (gitignored under .zcp/).
func acquireDotenvLock(workingDir, setup string) (func(), error) {
	lockDir := filepath.Join(workingDir, ".zcp", "state", "locks")
	if err := os.MkdirAll(lockDir, 0700); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}
	lockPath := filepath.Join(lockDir, fmt.Sprintf("dotenv-%s.lock", setup))
	f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("acquire flock: %w", err)
	}
	return func() {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
	}, nil
}

//go:build !unix

package ops

// acquireDotenvLock is a no-op on non-Unix platforms. ZCP development
// targets Mac/Linux; Windows callers get unsynchronized concurrent
// access. If Windows support becomes a requirement, swap this stub
// for a LockFileEx-based implementation under x/sys/windows.
func acquireDotenvLock(workingDir, setup string) (func(), error) {
	return func() {}, nil
}

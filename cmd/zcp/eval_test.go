package main

import (
	"strings"
	"testing"
)

// TestResolveEvalWorkDir pins the local-mode loud-fail gate. CleanupProject
// does rm -rf on workDir between scenarios; defaulting to /var/www on a
// developer machine fails confusingly when the path is missing and is
// actively destructive when an unrelated /var/www exists. The gate forces
// the operator to set ZCP_EVAL_WORK_DIR explicitly outside the zcp
// container.
func TestResolveEvalWorkDir(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		envValue    string
		inContainer bool
		wantDir     string
		wantOK      bool
		wantMsgSub  string
	}{
		{
			name:        "container without env defaults to /var/www",
			envValue:    "",
			inContainer: true,
			wantDir:     "/var/www",
			wantOK:      true,
		},
		{
			name:        "container with env override accepts",
			envValue:    "/custom/www",
			inContainer: true,
			wantDir:     "/custom/www",
			wantOK:      true,
		},
		{
			name:        "local with env accepts",
			envValue:    "/tmp/zcp-eval-workdir",
			inContainer: false,
			wantDir:     "/tmp/zcp-eval-workdir",
			wantOK:      true,
		},
		{
			name:        "local without env fails loud",
			envValue:    "",
			inContainer: false,
			wantOK:      false,
			wantMsgSub:  "ZCP_EVAL_WORK_DIR",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir, ok, msg := resolveEvalWorkDir(tt.envValue, tt.inContainer)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v (msg: %s)", ok, tt.wantOK, msg)
			}
			if ok && dir != tt.wantDir {
				t.Errorf("dir = %q, want %q", dir, tt.wantDir)
			}
			if !ok && tt.wantMsgSub != "" && !strings.Contains(msg, tt.wantMsgSub) {
				t.Errorf("msg should contain %q; got: %s", tt.wantMsgSub, msg)
			}
		})
	}
}

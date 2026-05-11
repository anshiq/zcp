package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/zeropsio/zcp/internal/ops"
	"github.com/zeropsio/zcp/internal/topology"
)

// launchState is the on-disk record persisted under
// .zcp/state/launch-production/{launchID}.json. Survives compaction +
// process restart. Used by handleLaunchProduction to recover mid-launch
// state and provide idempotent resume.
//
// P-LP-1 invariant: the launchKey is NEVER written here. The struct has
// no field for it. Tests pin via TestLaunchState_NoLaunchKeyFieldExists.
type launchState struct {
	LaunchID          string                                   `json:"launchID"`
	SourceProjectID   string                                   `json:"sourceProjectID"`
	TargetProjectID   string                                   `json:"targetProjectID,omitempty"`
	TargetProjectName string                                   `json:"targetProjectName"`
	ImportedServices  []importedServiceEntry                   `json:"importedServices,omitempty"`
	SourceSnapshot    ops.SourceSnapshot                       `json:"sourceSnapshot"`
	Classifications   map[string]topology.SecretClassification `json:"classifications,omitempty"`
	Status            topology.LaunchProductionStatus          `json:"status"`
	// CreatedAt is the moment launchID was first written.
	CreatedAt time.Time `json:"createdAt"`
	// LastUpdate is the latest mutation timestamp.
	LastUpdate time.Time `json:"lastUpdate"`
	// LastError carries the structured failure reason when Status=failed.
	// Excludes launchKey unconditionally.
	LastError string `json:"lastError,omitempty"`
}

// importedServiceEntry records one service stack created by
// CreateAndImportProject — used by status calls to know what to poll.
type importedServiceEntry struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	ProcessIDs []string `json:"processIDs,omitempty"`
	// ImportError, if non-empty, indicates this service's import had a
	// per-service error (still part of the same ImportResult — the API
	// returns per-service errors alongside successful entries).
	ImportError string `json:"importError,omitempty"`
}

// launchStateDir is the subdirectory under stateDir where launch state
// files live. One file per launchID.
const launchStateDir = "launch-production"

// generateLaunchID derives a deterministic launchID from
// (sourceProjectID, targetProjectName). Same inputs → same launchID so
// retries find the existing state file. Hash truncated to 16 hex chars
// (8 bytes) for human-readable file names.
func generateLaunchID(sourceProjectID, targetProjectName string) string {
	sum := sha256.Sum256([]byte(sourceProjectID + "::" + targetProjectName))
	return hex.EncodeToString(sum[:8])
}

// launchStatePath returns the absolute path to the state file for a
// given launchID under the configured stateDir.
func launchStatePath(stateDir, launchID string) string {
	return filepath.Join(stateDir, launchStateDir, launchID+".json")
}

// readLaunchState reads + decodes the state file. Returns (nil, nil) when
// the file doesn't exist (first invocation). Other errors propagate.
func readLaunchState(stateDir, launchID string) (*launchState, error) {
	path := launchStatePath(stateDir, launchID)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read launch state %s: %w", path, err)
	}
	var s launchState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("decode launch state %s: %w", path, err)
	}
	return &s, nil
}

// writeLaunchState marshals the state and atomically writes it to disk.
// Uses temp-file-and-rename for crash safety.
func writeLaunchState(stateDir string, state *launchState) error {
	if state == nil {
		return errors.New("write launch state: nil state")
	}
	if state.LaunchID == "" {
		return errors.New("write launch state: missing LaunchID")
	}
	dir := filepath.Join(stateDir, launchStateDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	state.LastUpdate = time.Now().UTC()
	if state.CreatedAt.IsZero() {
		state.CreatedAt = state.LastUpdate
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal launch state: %w", err)
	}
	finalPath := launchStatePath(stateDir, state.LaunchID)
	tmpPath := finalPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename to final: %w", err)
	}
	return nil
}

// launchAuditLogPath returns the path to the append-only audit log.
// One log per stateDir (per project context), shared across all launchIDs.
const launchAuditLogName = "launch-audit-log.json"

// launchAuditEntry is one append-only record of a launch mutation.
// Records who-did-what when, with no secret values and no launchKey.
type launchAuditEntry struct {
	Timestamp         time.Time                                `json:"timestamp"`
	LaunchID          string                                   `json:"launchID"`
	Action            string                                   `json:"action"` // e.g. "create-and-import", "delete-target", "publish-failed"
	SourceProjectID   string                                   `json:"sourceProjectID"`
	TargetProjectID   string                                   `json:"targetProjectID,omitempty"`
	TargetProjectName string                                   `json:"targetProjectName,omitempty"`
	SourceCommitSHA   string                                   `json:"sourceCommitSHA,omitempty"`
	SourceYAMLSHA256  string                                   `json:"sourceYAMLSHA256,omitempty"`
	Classifications   map[string]topology.SecretClassification `json:"classifications,omitempty"`
	HAOptOut          []string                                 `json:"haOptOut,omitempty"`
	Result            string                                   `json:"result"` // success | failure
	ErrorMessage      string                                   `json:"errorMessage,omitempty"`
}

// appendAuditLog appends one entry to the launch audit log. Open mode is
// O_APPEND so concurrent writes from parallel invocations don't clobber
// each other.
func appendAuditLog(stateDir string, entry launchAuditEntry) error {
	dir := filepath.Join(stateDir, launchStateDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, launchAuditLogName)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer func() { _ = f.Close() }()
	entry.Timestamp = time.Now().UTC()
	// One JSON object per line for easy tailing.
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal audit entry: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write audit entry: %w", err)
	}
	return nil
}

// sortedClassificationKeys returns the keys of a classification map in
// stable order — used by tests that diff classifications between calls.
func sortedClassificationKeys(m map[string]topology.SecretClassification) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

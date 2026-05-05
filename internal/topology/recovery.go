package topology

// Recovery is the structured next-tool pointer attached to failing checks
// and rejected operations so the agent has an explicit, machine-readable
// recovery surface.
//
// Lives in topology because both ops and workflow produce Recovery values
// (verify checks, workflow_checks status rejections, deploy preflight
// gates, dev_server pre-spawn gates) and tools.RecoveryHint is just the
// wire-form alias of this type. Promoting from per-layer mirrors to one
// foundational vocabulary type removes the boilerplate of "ops has its
// own Recovery, tools has its own RecoveryHint, both with identical
// JSON tags" — they are now the same type.
type Recovery struct {
	Tool   string            `json:"tool"`
	Action string            `json:"action"`
	Args   map[string]string `json:"args,omitempty"`
}

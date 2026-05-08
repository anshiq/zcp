// zcp-recipe-sim runs zcprecipator3 simulations against a frozen run.
//
// Subcommands:
//
//	emit             stage a frozen scaffold output + emit cc/env dispatch prompts
//	emit-finalize    compose finalize sub-agent prompt against the stitched corpus
//	emit-refinement  compose refinement sub-agent prompt against the stitched corpus
//	stitch           assemble fragments into the simulated recipe shape
//	validate         run slot-shape refusals over fragments
//
// End-to-end loop (mirrors production zcprecipator3 pipeline order):
//
//	zcp-recipe-sim emit -run docs/zcprecipator3/runs/23 \
//	    -out docs/zcprecipator3/simulations/24
//	# user dispatches N+1 Agent calls (per-codebase codebase-content + env-content);
//	# CLAUDE.md is auto-copied from <run>/<host>dev/CLAUDE.md (claudemd-author skipped).
//	zcp-recipe-sim emit-finalize   -dir docs/zcprecipator3/simulations/24
//	# user dispatches 1 finalize Agent call against briefs/finalize-prompt.md
//	zcp-recipe-sim stitch          -dir docs/zcprecipator3/simulations/24
//	zcp-recipe-sim emit-refinement -dir docs/zcprecipator3/simulations/24
//	# user dispatches 1 refinement Agent call against briefs/refinement-prompt.md
//	# (the dispatch pointer reads <dir>/.briefs/refinement-phase/index.md and
//	# walks every part-*.md listed in its "Read order" section — multi-file
//	# shape mirrors production refinement dispatch).
//	zcp-recipe-sim stitch          -dir docs/zcprecipator3/simulations/24
//	zcp-recipe-sim validate        -dir docs/zcprecipator3/simulations/24
//
// Stitch load order is [<codebase hosts>, env, finalize, refinement];
// refinement-phase fragments override prior phases at the same fragment
// id (last-write-wins). The emit step is byte-identical to the
// production engine's `zerops_recipe action=build-subagent-prompt`
// output (plus a 20-line replay adapter that redirects record-fragment
// to file-write). Brief or atom edits land identically in simulation
// and production — divergence lives only in the leading adapter.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	sub := os.Args[1]
	args := os.Args[2:]
	var err error
	switch sub {
	case "emit":
		err = runEmit(args)
	case "emit-finalize":
		err = runEmitFinalize(args)
	case "emit-refinement":
		err = runEmitRefinement(args)
	case "stitch":
		err = runStitch(args)
	case "validate":
		err = runValidate(args)
	case "help", "-h", "--help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", sub)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `zcp-recipe-sim — zcprecipator3 simulation tool

Canonical pipeline order (mirrors production zcprecipator3):

  zcp-recipe-sim emit            -run <run> -out <out> [-mount-root ...] [-parent ...]
  # dispatch N+1 cc + env Agent calls (per the briefs/<host>-prompt.md outputs)
  zcp-recipe-sim emit-finalize   -dir <out> [-mount-root ...] [-parent ...]
  # dispatch 1 finalize Agent call (against briefs/finalize-prompt.md)
  zcp-recipe-sim stitch          -dir <out>
  zcp-recipe-sim emit-refinement -dir <out> [-mount-root ...] [-parent ...]
  # dispatch 1 refinement Agent call (against briefs/refinement-prompt.md)
  zcp-recipe-sim stitch          -dir <out>    # re-stitch folds refinement
  zcp-recipe-sim validate        -dir <out>

Subcommands:
  emit      Stage frozen scaffold output + emit dispatch prompts.
            Auto-copies <run>/<host>dev/CLAUDE.md → fragments-new/<host>/
            (claudemd-author phase is policy-skipped in sim).
            Reads:  <run>/environments/{plan.json,facts.jsonl}
                    <run>/<host>dev/{zerops.yaml, CLAUDE.md, src/**, ...}
            Writes: <out>/environments/{plan.json,facts.jsonl}
                    <out>/<host>dev/zerops.yaml (bare) + staged code artifacts
                    <out>/briefs/{<host>,env}-prompt.md
                    <out>/fragments-new/<host>/codebase__<host>__claude-md.md
                    <out>/fragments-new/{<host>,env}/  (otherwise empty)
            Flags:  -run, -out, -mount-root, -parent

  emit-finalize  Compose the finalize sub-agent prompt against the
                 staged simulation. Run after the codebase-content +
                 env-content sub-agents authored fragments. In sim,
                 env-content already covers root/intro + per-tier
                 import-comments/project; finalize is mostly defensive.
                 Reads:  <dir>/environments/{plan.json,facts.jsonl}
                 Writes: <dir>/briefs/finalize-prompt.md
                         <dir>/fragments-new/finalize/  (empty)
                 Flags:  -dir, -mount-root, -parent

  stitch    Assemble simulated recipe from authored fragments. Load
            order is [<codebase hosts>, env, finalize, refinement] —
            refinement fragments override prior phases at the same id.
            Reads:  <dir>/environments/plan.json
                    <dir>/<host>dev/zerops.yaml (bare)
                    <dir>/fragments-new/{<host>,env,finalize,refinement}/*.md
            Writes: <dir>/README.md  (root)
                    <dir>/environments/<N>/{import.yaml,README.md}
                    <dir>/<host>dev/{README.md,CLAUDE.md,zerops.yaml}
            Flags:  -dir, -rounds, -gates

  emit-refinement  Compose the refinement sub-agent prompt against the
                   stitched simulation. Run after the codebase-content
                   + env-content sub-agents have authored fragments and
                   `+"`stitch`"+` has assembled the full corpus.
                   Multi-file brief shape (mirrors production): index.md
                   + N part-*.md persisted under .briefs/refinement-
                   phase/, with briefs/refinement-prompt.md as a thin
                   dispatch pointer (replay-adapter + index path).
                   Reads:  <dir>/environments/{plan.json,facts.jsonl}
                           <dir>/{README.md, environments/*, *dev/*}
                   Writes: <dir>/briefs/refinement-prompt.md  (pointer)
                           <dir>/.briefs/refinement-phase/{index.md, part-*.md}
                           <dir>/fragments-new/refinement/  (empty)
                   Flags:  -dir, -mount-root, -parent

  validate  Run slot-shape refusals over fragments.
            Reads:  <dir>/environments/plan.json
                    <dir>/fragments-new/<host>/*.md
            Flags:  -dir

`)
}

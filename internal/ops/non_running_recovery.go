package ops

import (
	"context"

	"github.com/zeropsio/zcp/internal/platform"
	"github.com/zeropsio/zcp/internal/topology"
)

// NonRunningRecovery returns the canonical Recovery hint for a service in a
// non-running terminal state, or nil when the status is intentional / pre-
// deploy / healthy. The discriminator for READY_TO_DEPLOY is "service has
// failed appVersion history" — never-deployed services point at logs (rare
// case, no diagnostic data yet); services that DID deploy and failed point
// at the import override (replace-and-redeploy is the recovery).
//
//	READY_TO_DEPLOY + LatestFailedAppVersionContext != nil → zerops_import
//	    override=true startWithoutCode=true (the agent's first call hits
//	    the Phase 3.2 confirmDestructive gate, gets a structured loss
//	    payload back, then re-calls with the acknowledgment)
//	READY_TO_DEPLOY + no failed history → zerops_logs (never-deployed)
//	FAILED → zerops_events (classified failure via events timeline)
//	STOPPED / NEW / RUNNING / ACTIVE → nil (intentional / pre-deploy /
//	    healthy — no recovery candidate)
//
// fetcher is forwarded into LatestFailedAppVersionContext for log
// enrichment; pass nil when the caller doesn't have one (workflow_checks
// lifecycle gate runs without log access). Per plan v4 §1.4.
func NonRunningRecovery(
	ctx context.Context,
	client platform.Client,
	fetcher platform.LogFetcher,
	projectID, hostname, status string,
) *topology.Recovery {
	switch status {
	case platform.ServiceStatusReadyToDeploy:
		failed, _ := LatestFailedAppVersionContext(ctx, client, fetcher, projectID, hostname)
		if failed != nil {
			return &topology.Recovery{
				Tool:   "zerops_import",
				Action: "import",
				Args: map[string]string{
					"override":         "true",
					"startWithoutCode": "true",
				},
			}
		}
		return &topology.Recovery{
			Tool:   "zerops_logs",
			Action: "fetch",
			Args: map[string]string{
				"serviceHostname": hostname,
				"facility":        logFacilityApplication,
				"since":           "15m",
			},
		}
	case platform.ServiceStatusFailed:
		return &topology.Recovery{
			Tool:   "zerops_events",
			Action: "fetch",
			Args: map[string]string{
				"serviceHostname": hostname,
			},
		}
	}
	return nil
}

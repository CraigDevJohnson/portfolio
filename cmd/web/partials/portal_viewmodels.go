package partials

// PortalState is the closed set of EC2 lifecycle states with known operator
// semantics. Raw AWS state remains available separately for diagnostics.
type PortalState string

const (
	PortalStatePending      PortalState = "pending"
	PortalStateRunning      PortalState = "running"
	PortalStateStopping     PortalState = "stopping"
	PortalStateStopped      PortalState = "stopped"
	PortalStateShuttingDown PortalState = "shutting-down"
	PortalStateTerminated   PortalState = "terminated"
)

// PortalStateView is the safe presentation for a raw EC2 lifecycle state.
type PortalStateView struct {
	Class       string
	Label       string
	Description string
}

func portalStatePresentation(state string) PortalStateView {
	switch PortalState(state) {
	case PortalStatePending:
		return PortalStateView{
			Class:       "portal-state-pending",
			Label:       "Pending",
			Description: "AWS is preparing this instance; controls remain unavailable until its state settles.",
		}
	case PortalStateRunning:
		return PortalStateView{
			Class:       "portal-state-running",
			Label:       "Running",
			Description: "This instance is online and can be stopped or restarted.",
		}
	case PortalStateStopping:
		return PortalStateView{
			Class:       "portal-state-stopping",
			Label:       "Stopping",
			Description: "AWS is stopping this instance; controls remain unavailable until it is stopped.",
		}
	case PortalStateStopped:
		return PortalStateView{
			Class:       "portal-state-stopped",
			Label:       "Stopped",
			Description: "This instance is offline and can be started.",
		}
	case PortalStateShuttingDown:
		return PortalStateView{
			Class:       "portal-state-shutting-down",
			Label:       "Shutting down",
			Description: "AWS is permanently shutting down this instance; controls remain unavailable.",
		}
	case PortalStateTerminated:
		return PortalStateView{
			Class:       "portal-state-terminated",
			Label:       "Terminated",
			Description: "This instance has been terminated and can no longer be managed.",
		}
	default:
		return PortalStateView{
			Class:       "portal-state-unknown",
			Label:       "Unknown",
			Description: "This lifecycle state is not recognized, so instance actions remain unavailable.",
		}
	}
}

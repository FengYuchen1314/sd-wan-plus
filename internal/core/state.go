package core

// RolloutPhase models Prepare → Activate → Verify for a single node.
type RolloutPhase string

const (
	PhaseIdle       RolloutPhase = "idle"
	PhasePrepare    RolloutPhase = "prepare"
	PhaseActivate   RolloutPhase = "activate"
	PhaseVerify     RolloutPhase = "verify"
	PhaseRollback   RolloutPhase = "rollback"
	PhaseDone       RolloutPhase = "done"
	PhaseFailed     RolloutPhase = "failed"
)

// NextRolloutStatus advances a node rollout status on success/failure.
func NextRolloutStatus(current string, success bool, phase RolloutPhase) string {
	if !success {
		switch phase {
		case PhasePrepare:
			return RolloutPrepareFailed
		case PhaseActivate:
			return RolloutActivateFailed
		case PhaseVerify:
			return RolloutVerifyFailed
		case PhaseRollback:
			return RolloutRolledBack
		default:
			return RolloutPrepareFailed
		}
	}
	switch current {
	case RolloutPending:
		return RolloutDispatched
	case RolloutDispatched:
		return RolloutPreparing
	case RolloutPreparing:
		return RolloutPrepared
	case RolloutPrepared:
		return RolloutActivating
	case RolloutActivating:
		return RolloutActive
	case RolloutPrepareFailed, RolloutActivateFailed, RolloutVerifyFailed:
		return RolloutRollingBack
	case RolloutRollingBack:
		return RolloutRolledBack
	default:
		return current
	}
}

// UpdatePhase models the updater state machine.
type UpdatePhase string

const (
	UpdatePhaseWait    UpdatePhase = "wait"
	UpdatePhaseFetch   UpdatePhase = "fetch"
	UpdatePhaseVerify  UpdatePhase = "verify"
	UpdatePhaseStage   UpdatePhase = "stage"
	UpdatePhaseInstall UpdatePhase = "install"
	UpdatePhaseRestart UpdatePhase = "restart"
	UpdatePhaseHealth  UpdatePhase = "health"
	UpdatePhaseDone    UpdatePhase = "done"
)

func NextUpdateStatus(current string, success bool, phase UpdatePhase) string {
	if !success {
		switch phase {
		case UpdatePhaseFetch:
			return UpdateDownloadFailed
		case UpdatePhaseVerify:
			return UpdateSignatureInvalid
		case UpdatePhaseInstall, UpdatePhaseRestart:
			return UpdateInstallFailed
		case UpdatePhaseHealth:
			return UpdateHealthFailed
		default:
			return UpdateInstallFailed
		}
	}
	switch current {
	case UpdateWaiting:
		return UpdatePrefetching
	case UpdatePrefetching:
		return UpdateVerifying
	case UpdateVerifying:
		return UpdateStaged
	case UpdateStaged:
		return UpdateInstalling
	case UpdateInstalling:
		return UpdateRestarting
	case UpdateRestarting:
		return UpdateHealthChecking
	case UpdateHealthChecking:
		return UpdateCompleted
	case UpdateDownloadFailed, UpdateSignatureInvalid, UpdateInstallFailed, UpdateHealthFailed:
		return UpdateRollingBack
	case UpdateRollingBack:
		return UpdateRolledBack
	default:
		return current
	}
}

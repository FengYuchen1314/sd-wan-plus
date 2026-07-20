use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum ConfigRolloutState {
    Pending,
    Dispatched,
    Preparing,
    Prepared,
    Activating,
    Active,
    PrepareFailed,
    ActivateFailed,
    VerifyFailed,
    RollingBack,
    RolledBack,
}

impl ConfigRolloutState {
    pub fn is_terminal(&self) -> bool {
        matches!(self, Self::Active | Self::RolledBack)
    }

    pub fn is_error(&self) -> bool {
        matches!(
            self,
            Self::PrepareFailed | Self::ActivateFailed | Self::VerifyFailed
        )
    }

    pub fn can_transition_to(&self, next: &Self) -> bool {
        match (self, next) {
            (Self::Pending, Self::Dispatched) => true,
            (Self::Dispatched, Self::Preparing) => true,
            (Self::Preparing, Self::Prepared) => true,
            (Self::Preparing, Self::PrepareFailed) => true,
            (Self::Prepared, Self::Activating) => true,
            (Self::Activating, Self::Active) => true,
            (Self::Activating, Self::ActivateFailed) => true,
            (Self::Active, _) => false,
            (Self::PrepareFailed, Self::RollingBack) => true,
            (Self::ActivateFailed, Self::RollingBack) => true,
            (Self::VerifyFailed, Self::RollingBack) => true,
            (Self::RollingBack, Self::RolledBack) => true,
            (Self::RolledBack, Self::Pending) => true,
            _ => false,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum UpdateTargetState {
    Waiting,
    Prefetching,
    Verifying,
    Staged,
    Installing,
    Restarting,
    HealthChecking,
    Completed,
    DownloadFailed,
    SignatureInvalid,
    InstallFailed,
    HealthCheckFailed,
    RollingBack,
    RolledBack,
}

impl UpdateTargetState {
    pub fn is_terminal(&self) -> bool {
        matches!(self, Self::Completed | Self::RolledBack)
    }

    pub fn is_error(&self) -> bool {
        matches!(
            self,
            Self::DownloadFailed
                | Self::SignatureInvalid
                | Self::InstallFailed
                | Self::HealthCheckFailed
        )
    }

    pub fn can_transition_to(&self, next: &Self) -> bool {
        match (self, next) {
            (Self::Waiting, Self::Prefetching) => true,
            (Self::Prefetching, Self::Verifying) => true,
            (Self::Prefetching, Self::DownloadFailed) => true,
            (Self::Verifying, Self::Staged) => true,
            (Self::Verifying, Self::SignatureInvalid) => true,
            (Self::Staged, Self::Installing) => true,
            (Self::Installing, Self::Restarting) => true,
            (Self::Installing, Self::InstallFailed) => true,
            (Self::Restarting, Self::HealthChecking) => true,
            (Self::HealthChecking, Self::Completed) => true,
            (Self::HealthChecking, Self::HealthCheckFailed) => true,
            (Self::DownloadFailed, Self::RollingBack) => true,
            (Self::SignatureInvalid, Self::RollingBack) => true,
            (Self::InstallFailed, Self::RollingBack) => true,
            (Self::HealthCheckFailed, Self::RollingBack) => true,
            (Self::RollingBack, Self::RolledBack) => true,
            (Self::RolledBack, Self::Waiting) => true,
            (Self::Completed, _) => false,
            _ => false,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum AgentState {
    Bootstrap,
    Registering,
    Registered,
    OverlayConnecting,
    OverlayConnected,
    OverlayDisconnected,
    Recovery,
    Updating,
    ShuttingDown,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum LinkLifecycle {
    Creating,
    ListenerPrepared,
    ListenerActive,
    InitiatorPrepared,
    InitiatorActive,
    HandshakeComplete,
    Verified,
    TeardownPending,
    TeardownComplete,
}

impl LinkLifecycle {
    pub fn can_transition_to(&self, next: &Self) -> bool {
        match (self, next) {
            (Self::Creating, Self::ListenerPrepared) => true,
            (Self::ListenerPrepared, Self::ListenerActive) => true,
            (Self::ListenerActive, Self::InitiatorPrepared) => true,
            (Self::InitiatorPrepared, Self::InitiatorActive) => true,
            (Self::InitiatorActive, Self::HandshakeComplete) => true,
            (Self::HandshakeComplete, Self::Verified) => true,
            (Self::Verified, Self::TeardownPending) => true,
            (Self::TeardownPending, Self::TeardownComplete) => true,
            _ => false,
        }
    }
}

use thiserror::Error;

#[derive(Error, Debug)]
pub enum PathWeaverError {
    #[error("Authentication failed: {0}")]
    AuthenticationFailed(String),

    #[error("Session expired")]
    SessionExpired,

    #[error("Session revoked")]
    SessionRevoked,

    #[error("Invalid session token")]
    InvalidSessionToken,

    #[error("Node not found: {0}")]
    NodeNotFound(String),

    #[error("Node is offline: {0}")]
    NodeOffline(String),

    #[error("Link not found: {0}")]
    LinkNotFound(String),

    #[error("Policy not found: {0}")]
    PolicyNotFound(String),

    #[error("Token expired")]
    TokenExpired,

    #[error("Token already used")]
    TokenAlreadyUsed,

    #[error("Token revoked")]
    TokenRevoked,

    #[error("Invalid token: {0}")]
    InvalidToken(String),

    #[error("Parent node not found: {0}")]
    ParentNotFound(String),

    #[error("Circular control tree detected")]
    CircularControlTree,

    #[error("Circular path detected")]
    CircularPath,

    #[error("Path segment missing link between {0} and {1}")]
    PathMissingLink(String, String),

    #[error("Config generation mismatch: node={0}, expected={1}")]
    ConfigGenerationMismatch(String, u64),

    #[error("Config prepare failed: {0}")]
    ConfigPrepareFailed(String),

    #[error("Config activate failed: {0}")]
    ConfigActivateFailed(String),

    #[error("Config verify failed for node {0}: {1}")]
    ConfigVerifyFailed(String, String),

    #[error("Config rollback failed: {0}")]
    ConfigRollbackFailed(String),

    #[error("WireGuard interface creation failed: {0}")]
    WireGuardError(String),

    #[error("Network is unreachable: {0}")]
    NetworkUnreachable(String),

    #[error("Duplicate display name: {0}")]
    DuplicateDisplayName(String),

    #[error("Artifact not found: {0}")]
    ArtifactNotFound(String),

    #[error("Artifact checksum mismatch: expected={0}, actual={1}")]
    ArtifactChecksumMismatch(String, String),

    #[error("Artifact signature invalid")]
    ArtifactSignatureInvalid,

    #[error("Update failed at node {0}: {1}")]
    UpdateFailed(String, String),

    #[error("Update health check failed at node {0}")]
    UpdateHealthCheckFailed(String),

    #[error("Update rollback failed at node {0}")]
    UpdateRollbackFailed(String),

    #[error("Protocol version mismatch: node={0}, expected={1}, actual={2}")]
    ProtocolVersionMismatch(String, u32, u32),

    #[error("Port conflict: port {0} already in use")]
    PortConflict(u16),

    #[error("No available port in pool")]
    NoAvailablePort,

    #[error("Database error: {0}")]
    DatabaseError(String),

    #[error("IO error: {0}")]
    IoError(String),

    #[error("Serialization error: {0}")]
    SerializationError(String),

    #[error("Internal error: {0}")]
    InternalError(String),

    #[error("Validation error: {0}")]
    ValidationError(String),

    #[error("Timeout: {0}")]
    Timeout(String),

    #[error("Conflict: {0}")]
    Conflict(String),
}

pub type Result<T> = std::result::Result<T, PathWeaverError>;

package ports

// Default control-plane and WireGuard port assignments (14301–14399).
const (
	Web     = 14301 // controller Web UI + admin API
	Node    = 14302 // node service: bootstrap, artifacts, agent APIs
	WGStart = 14303 // WireGuard UDP pool start
	WGEnd   = 14399 // WireGuard UDP pool end
)

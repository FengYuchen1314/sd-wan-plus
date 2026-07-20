package relay

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// Envelope is the control-plane relay message.
type Envelope struct {
	MessageID    string `json:"message_id"`
	SourceNodeID string `json:"source_node_id"`
	TargetNodeID string `json:"target_node_id"`
	NetworkID    string `json:"network_id"`
	TTL          uint32 `json:"ttl"`
	TraceID      string `json:"trace_id"`
	PayloadType  string `json:"payload_type"`
	Payload      []byte `json:"payload"`
}

func NewEnvelope(src, dst, network, payloadType string, payload []byte) *Envelope {
	return &Envelope{
		MessageID: uuid.NewString(), SourceNodeID: src, TargetNodeID: dst,
		NetworkID: network, TTL: 32, TraceID: uuid.NewString(),
		PayloadType: payloadType, Payload: payload,
	}
}

// Deduper prevents duplicate execution of relayed messages.
type Deduper struct {
	mu   sync.Mutex
	seen map[string]time.Time
	ttl  time.Duration
}

func NewDeduper(ttl time.Duration) *Deduper {
	return &Deduper{seen: make(map[string]time.Time), ttl: ttl}
}

func (d *Deduper) Seen(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	for k, t := range d.seen {
		if now.Sub(t) > d.ttl {
			delete(d.seen, k)
		}
	}
	if _, ok := d.seen[id]; ok {
		return true
	}
	d.seen[id] = now
	return false
}

func (e *Envelope) DecrementTTL() bool {
	if e.TTL == 0 {
		return false
	}
	e.TTL--
	return e.TTL > 0 || true
}

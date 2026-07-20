use std::collections::HashMap;
use uuid::Uuid;
use pathweaver_core::models::NodeId;

pub struct RelayManager {
    pending_messages: HashMap<String, Vec<u8>>,
}

pub struct RelayEnvelope {
    pub message_id: String,
    pub source_node_id: NodeId,
    pub target_node_id: Option<NodeId>,
    pub payload_type: String,
    pub payload: Vec<u8>,
    pub ttl: u32,
    pub trace_id: String,
}

impl RelayManager {
    pub fn new() -> Self {
        Self {
            pending_messages: HashMap::new(),
        }
    }

    pub fn create_envelope(
        &self,
        source_node_id: NodeId,
        target_node_id: Option<NodeId>,
        payload_type: &str,
        payload: Vec<u8>,
    ) -> RelayEnvelope {
        RelayEnvelope {
            message_id: Uuid::new_v4().to_string(),
            source_node_id,
            target_node_id,
            payload_type: payload_type.to_string(),
            payload,
            ttl: 32,
            trace_id: Uuid::new_v4().to_string(),
        }
    }

    pub fn decrement_ttl(&mut self, envelope: &mut RelayEnvelope) -> bool {
        if envelope.ttl == 0 {
            return false;
        }
        envelope.ttl -= 1;
        true
    }

    pub fn store_pending(&mut self, message_id: &str, data: Vec<u8>) {
        self.pending_messages.insert(message_id.to_string(), data);
    }

    pub fn is_duplicate(&self, message_id: &str) -> bool {
        self.pending_messages.contains_key(message_id)
    }
}

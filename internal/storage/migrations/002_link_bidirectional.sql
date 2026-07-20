-- One-way handshake by default; bidirectional only when explicitly enabled (public↔public).
ALTER TABLE wireguard_links ADD COLUMN bidirectional INTEGER NOT NULL DEFAULT 0;

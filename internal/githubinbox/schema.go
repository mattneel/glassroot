package githubinbox

const schemaSQL = `
CREATE TABLE IF NOT EXISTS github_metadata (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
) STRICT;

CREATE TABLE IF NOT EXISTS github_inbox (
  receiver_id TEXT NOT NULL,
  delivery_id TEXT NOT NULL,
  body_digest TEXT NOT NULL,
  intake_fingerprint TEXT NOT NULL,
  event_name TEXT NOT NULL,
  action TEXT,
  projection_kind TEXT NOT NULL,
  disposition TEXT NOT NULL,
  matched_secret TEXT NOT NULL,
  received_at TEXT NOT NULL,
  installation_id INTEGER,
  repository_id INTEGER,
  pull_request_number INTEGER,
  check_run_id INTEGER,
  base_sha TEXT,
  head_sha TEXT,
  receipt_json TEXT NOT NULL,
  projection_json TEXT NOT NULL,
  record_digest TEXT NOT NULL,
  conflict_count INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  PRIMARY KEY(receiver_id, delivery_id),
  UNIQUE(receiver_id, intake_fingerprint)
) STRICT;

CREATE TABLE IF NOT EXISTS github_outbox (
  sequence INTEGER PRIMARY KEY AUTOINCREMENT,
  id TEXT NOT NULL UNIQUE,
  receiver_id TEXT NOT NULL,
  delivery_id TEXT NOT NULL,
  projection_kind TEXT NOT NULL,
  projection_json TEXT NOT NULL,
  projection_digest TEXT NOT NULL,
  receipt_json TEXT NOT NULL,
  state TEXT NOT NULL,
  lease_owner TEXT,
  lease_generation INTEGER NOT NULL DEFAULT 0,
  lease_expires_at TEXT,
  attempt_count INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  acknowledged_at TEXT,
  last_failure_code TEXT,
  FOREIGN KEY(receiver_id, delivery_id) REFERENCES github_inbox(receiver_id, delivery_id) ON DELETE RESTRICT
) STRICT;

CREATE INDEX IF NOT EXISTS github_outbox_claim_idx ON github_outbox(state, sequence);
`

const insertInboxSQL = `INSERT OR IGNORE INTO github_inbox(receiver_id, delivery_id, body_digest, intake_fingerprint, event_name, action, projection_kind, disposition, matched_secret, received_at, installation_id, repository_id, pull_request_number, check_run_id, base_sha, head_sha, receipt_json, projection_json, record_digest, created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

const insertOutboxSQL = `INSERT INTO github_outbox(id, receiver_id, delivery_id, projection_kind, projection_json, projection_digest, receipt_json, state, created_at) VALUES(?,?,?,?,?,?,?,'pending',?)`

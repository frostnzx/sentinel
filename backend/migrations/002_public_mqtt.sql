CREATE TABLE IF NOT EXISTS events (
  id uuid PRIMARY KEY,
  source text NOT NULL,
  kind text NOT NULL,
  payload jsonb NOT NULL,
  recorded_at timestamptz NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS events_time ON events (recorded_at DESC);

CREATE TABLE IF NOT EXISTS face_sync (
  singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
  version bigint NOT NULL,
  model_version text NOT NULL,
  snapshot jsonb NOT NULL,
  applied_version bigint,
  error text NOT NULL DEFAULT '',
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE IF NOT EXISTS readings (
  id uuid NOT NULL,
  source text NOT NULL CHECK (source IN ('node','gateway')),
  door_open boolean,
  lock_state text,
  presence boolean,
  temperature_c double precision,
  distance_mm double precision,
  light_lux double precision,
  recorded_at timestamptz NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (id, recorded_at)
);
SELECT create_hypertable('readings', 'recorded_at', if_not_exists => TRUE);
CREATE INDEX IF NOT EXISTS readings_source_time ON readings (source, recorded_at DESC);

CREATE TABLE IF NOT EXISTS commands (
  id uuid PRIMARY KEY,
  action text NOT NULL CHECK (action IN ('lock','unlock')),
  status text NOT NULL CHECK (status IN ('pending','succeeded','failed','timed_out')),
  message text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL,
  expires_at timestamptz NOT NULL
);
SELECT add_retention_policy('readings', INTERVAL '30 days', if_not_exists => TRUE);

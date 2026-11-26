CREATE TABLE IF NOT EXISTS visitor_photos (
  id uuid PRIMARY KEY,
  event_id uuid UNIQUE NOT NULL,
  content_type text NOT NULL,
  image bytea NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  analysis_status text,
  analysis_result jsonb,
  analysis_started_at timestamptz
);

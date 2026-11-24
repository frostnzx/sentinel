ALTER TABLE readings ADD COLUMN IF NOT EXISTS samples jsonb;
ALTER TABLE readings DROP CONSTRAINT IF EXISTS readings_source_check;
ALTER TABLE readings ADD CONSTRAINT readings_source_check CHECK (source IN ('node','gateway','combined'));
CREATE UNIQUE INDEX IF NOT EXISTS readings_combined_interval ON readings (recorded_at) WHERE source = 'combined';

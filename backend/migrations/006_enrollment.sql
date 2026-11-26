CREATE TABLE IF NOT EXISTS trusted_people (id uuid PRIMARY KEY, name text NOT NULL, deleted_version bigint, created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS face_enrollments (id uuid PRIMARY KEY, person_id uuid NOT NULL REFERENCES trusted_people(id), status text NOT NULL, results jsonb, error text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now());

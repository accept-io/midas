ALTER TABLE operational_envelopes
    ADD COLUMN IF NOT EXISTS explanation_json JSONB;
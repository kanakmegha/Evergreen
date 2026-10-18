-- Everroot anonymous discovery sessions.
--
-- Minimal, short-lived session state for the conversational discovery flow.
-- No names, birth dates, birth times, or precise locations are stored.
-- Full chat transcripts are not persisted; only structured fields needed to
-- resume a discovery and link it to its verified result.
--
-- Sessions expire automatically (24h default) and are cleaned up by the API.

CREATE TABLE discovery_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    intent text,
    tradition_slug text,
    region_name text,
    nakshatra text,
    state text NOT NULL DEFAULT 'greeting' CHECK (state IN (
        'greeting',
        'intent_selected',
        'collecting_details',
        'ready_for_discovery',
        'discovery_complete',
        'no_verified_mapping'
    )),
    discovery_mapping_id uuid,
    discovery_revision_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT now() + interval '24 hours'
);

CREATE INDEX discovery_sessions_expires_idx ON discovery_sessions(expires_at);

-- Clean up expired sessions.
CREATE OR REPLACE FUNCTION cleanup_expired_sessions() RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    DELETE FROM discovery_sessions WHERE expires_at < now();
END;
$$;

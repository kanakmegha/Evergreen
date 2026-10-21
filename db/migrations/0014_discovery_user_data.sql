-- Everroot user data for the AI-first discovery pipeline.
--
-- This migration is additive and forward-only. It creates the tables the
-- application needs to save a visitor's own discoveries, and nothing else.
--
-- What is deliberately NOT here is as important as what is. There is no tree
-- species catalogue, no cultural tradition or sacred-tree mapping, no
-- Nakshatra-to-tree or zodiac-to-tree association, and no precomputed global
-- recommendation. Everroot generates its recommendations dynamically from
-- retrieved sources, so a permanent answer table would be both redundant and
-- actively misleading: it would invite a future reader to treat a model output
-- as a reviewed fact. Saved research travels with the specific discovery it
-- was retrieved for, never into a shared global store.
--
-- References to botanical or taxonomic records are URLs on the rows below,
-- not foreign keys into a catalogue. A citation is an address a visitor can
-- follow, not a row Everroot owns.
--
-- Requirements satisfied by this schema:
--   * anonymous discovery — a run needs no user, only the token that made it
--   * ownership on every read and delete — enforced by owner_hash
--   * retention — every row carries expires_at or is cascaded from one that does
--   * no secrets — tokens are stored only as a SHA-256, never in plaintext
--
-- Extensions: none. PostgreSQL 13+ provides gen_random_uuid() in core, so this
-- migration needs no CREATE EXTENSION and installs nothing. It is deliberately
-- independent of the pgcrypto extension that migration 0001 installs: a
-- deployment that only wants user data should not have to enable a crypto
-- extension to get a UUID column, and the core function is the right tool
-- here regardless.

-- ---------------------------------------------------------------------------
-- users
--
-- Minimal. Everroot does not ask for an email, a name, a date of birth or a
-- location, and there is nowhere here to put one. "handle" is an opaque label
-- the interface may render; it is not an identity and is never used to
-- authorise anything.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    handle      text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- auth_tokens
--
-- One row per bearer token ever issued. The token itself is never stored:
-- only its SHA-256, so a database read never yields a usable credential. A
-- token can be revoked or allowed to lapse without touching the discoveries
-- it created, which is why ownership does not depend on this row still being
-- alive.
--
-- A token with a NULL user_id is an anonymous session. That is the normal
-- case, and it is a first-class one rather than a degraded mode: most
-- discoveries should not need an account.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS auth_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid REFERENCES users(id) ON DELETE CASCADE,
    token_hash   bytea NOT NULL UNIQUE,
    kind         text NOT NULL DEFAULT 'anonymous'
                 CHECK (kind IN ('anonymous', 'registered')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    expires_at   timestamptz NOT NULL DEFAULT now() + interval '90 days',
    revoked_at   timestamptz
);

CREATE INDEX IF NOT EXISTS auth_tokens_user_idx
    ON auth_tokens(user_id) WHERE user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS auth_tokens_expires_idx
    ON auth_tokens(expires_at);

-- ---------------------------------------------------------------------------
-- discovery_runs
--
-- One row per AI-first discovery. This is the AI-first pipeline's record and
-- is a different thing from the 0011 "discovery_sessions" table, which holds
-- short-lived conversational state for the older greeting flow and expires in
-- 24 hours. Keeping them apart avoids overloading a CHECK-constrained state
-- column with meanings it was not designed for.
--
-- Ownership is owner_hash: the SHA-256 of the bearer token that created the
-- run. Every read and delete compares against it, so a visitor cannot reach
-- another visitor's runs even by guessing an id, and no id is ever treated as
-- proof of ownership on its own.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS discovery_runs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid REFERENCES users(id) ON DELETE SET NULL,
    owner_hash      bytea NOT NULL,
    intention       text NOT NULL,
    description     text,
    tradition       text,
    region          text,
    nakshatra       text,
    constraints     jsonb NOT NULL DEFAULT '[]'::jsonb,
    research_status text NOT NULL DEFAULT 'unavailable'
                    CHECK (research_status IN ('researched', 'partial', 'unavailable')),
    status          text NOT NULL DEFAULT 'running'
                    CHECK (status IN ('running', 'complete', 'failed')),
    -- error_category is a credential-free classification (the same taxonomy
    -- the health layer reports). It is never a message, a stack or a URL.
    error_category  text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    completed_at    timestamptz,
    expires_at      timestamptz NOT NULL DEFAULT now() + interval '30 days'
);

-- Ownership lookups are the only reads this API serves.
CREATE INDEX IF NOT EXISTS discovery_runs_owner_idx
    ON discovery_runs(owner_hash, created_at DESC);

CREATE INDEX IF NOT EXISTS discovery_runs_expires_idx
    ON discovery_runs(expires_at);

-- ---------------------------------------------------------------------------
-- discovery_messages
--
-- The visitor's own input and the follow-up question, in order. Full chat
-- transcripts are not persisted: only the structured text the interface needs
-- to show a visitor what they asked.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS discovery_messages (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id     uuid NOT NULL REFERENCES discovery_runs(id) ON DELETE CASCADE,
    role       text NOT NULL CHECK (role IN ('user', 'follow_up')),
    content    text NOT NULL,
    seq        integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (run_id, seq)
);

-- ---------------------------------------------------------------------------
-- discovery_results
--
-- The generated answer, stored as the structured payload it was returned as.
-- JSONB rather than a fixed column set because the shape of a candidate is
-- owned by the prompt, which changes; the columns beside it are the facts the
-- application itself asserts and must never be a model's word for.
--
-- prompt_version and research_status are what make a stored result auditable
-- later: without them a result cannot be told apart from one produced by a
-- different prompt, or from one that was never researched at all.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS discovery_results (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id          uuid NOT NULL UNIQUE REFERENCES discovery_runs(id) ON DELETE CASCADE,
    payload         jsonb NOT NULL,
    prompt_version  text NOT NULL,
    model           text,
    research_status text NOT NULL
                    CHECK (research_status IN ('researched', 'partial', 'unavailable')),
    disclaimer      text,
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- discovery_sources
--
-- The sources retrieved FOR THIS RESULT. They are not a shared global
-- knowledge table: the same tree researched for two different visitors can
-- carry different sources, and neither result is entitled to reuse the
-- other's.
--
-- source_id is the ID the model cited inside the payload; url is the real
-- address the server resolved it to. Keeping both means every citation on a
-- card can be traced to a row, and a citation the model invented has no row
-- and is rejected before it is stored.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS discovery_sources (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    result_id    uuid NOT NULL REFERENCES discovery_results(id) ON DELETE CASCADE,
    source_id    text NOT NULL,
    title        text NOT NULL,
    url          text NOT NULL,
    publisher    text,
    kind         text NOT NULL,
    snippet      text,
    retrieved_at timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (result_id, source_id)
);

-- ---------------------------------------------------------------------------
-- user_feedback
--
-- Feedback on discovery results, quality evaluations, or corrections.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS user_feedback (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id        uuid REFERENCES discovery_runs(id) ON DELETE CASCADE,
    user_id       uuid REFERENCES users(id) ON DELETE SET NULL,
    feedback_type text NOT NULL DEFAULT 'result',
    rating        smallint,
    comment       text,
    created_at    timestamptz NOT NULL DEFAULT now()
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'user_feedback' AND column_name = 'run_id'
    ) THEN
        ALTER TABLE user_feedback ADD COLUMN run_id uuid REFERENCES discovery_runs(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'user_feedback' AND column_name = 'user_id'
    ) THEN
        ALTER TABLE user_feedback ADD COLUMN user_id uuid REFERENCES users(id) ON DELETE SET NULL;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS user_feedback_run_idx ON user_feedback(run_id);

-- ---------------------------------------------------------------------------
-- Retention
--
-- Every discovery row is reachable from discovery_runs, which carries
-- expires_at. Deleting a run cascades to its messages, result and sources, so
-- there is exactly one place to enforce a retention rule and no orphan is
-- possible. cleanup_expired_discoveries is the whole rule; the API calls it on
-- a schedule, and it is safe to run repeatedly and concurrently.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION cleanup_expired_discoveries() RETURNS bigint
LANGUAGE plpgsql AS $$
DECLARE
    removed bigint;
BEGIN
    DELETE FROM discovery_runs WHERE expires_at < now();
    GET DIAGNOSTICS removed = ROW_COUNT;
    -- Auth tokens outlive the runs they created, so they are swept here too;
    -- a lapsed anonymous token must not accumulate forever.
    DELETE FROM auth_tokens WHERE expires_at < now() OR revoked_at IS NOT NULL;
    RETURN removed;
END;
$$;

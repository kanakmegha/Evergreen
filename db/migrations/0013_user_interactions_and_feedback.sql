-- Everroot User Interactions and Feedback Persistence
--
-- Stores user questions and AI-generated answers for quality auditing,
-- prompt version evaluation, and user feedback (helpful/unhelpful ratings,
-- comments, and corrections).
--
-- User questions are answered via the external AI API and are never dependent
-- on reading this table to generate answers.

CREATE TABLE IF NOT EXISTS user_interactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id TEXT,
    question TEXT NOT NULL,
    answer TEXT NOT NULL,
    prompt_version TEXT NOT NULL,
    model TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_user_interactions_created_at ON user_interactions (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_user_interactions_session_id ON user_interactions (session_id);

CREATE TABLE IF NOT EXISTS user_feedback (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    interaction_id UUID REFERENCES user_interactions(id) ON DELETE SET NULL,
    feedback_type TEXT NOT NULL DEFAULT 'answer',
    rating SMALLINT,
    comment TEXT,
    correction TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_user_feedback_created_at ON user_feedback (created_at DESC);

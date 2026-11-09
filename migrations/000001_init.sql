-- gen_random_uuid() is built into PostgreSQL 13+; pgcrypto covers older versions.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           VARCHAR(255) UNIQUE,
    target_title    VARCHAR(150) NOT NULL,
    target_location VARCHAR(100) NOT NULL,
    resume_text     TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS hits (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    content_hash VARCHAR(64) UNIQUE NOT NULL,
    title        VARCHAR(255) NOT NULL,
    company      VARCHAR(255) NOT NULL,
    location     VARCHAR(150) NOT NULL,
    description  TEXT NOT NULL,
    source_url   TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS jobs (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    hit_id     UUID NOT NULL REFERENCES hits(id) ON DELETE CASCADE,
    score      SMALLINT NOT NULL CHECK (score >= 0 AND score <= 100),
    verdict    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_user_hit UNIQUE (user_id, hit_id)
);

CREATE INDEX IF NOT EXISTS idx_hits_location ON hits(location);
CREATE INDEX IF NOT EXISTS idx_hits_created_at ON hits(created_at);
CREATE INDEX IF NOT EXISTS idx_jobs_user_feed ON jobs(user_id, score DESC);

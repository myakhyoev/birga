CREATE TABLE IF NOT EXISTS activities (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title_uz         TEXT        NOT NULL,
    title_ru         TEXT        NOT NULL,
    description_uz   TEXT        NOT NULL,
    description_ru   TEXT        NOT NULL,
    goal             TEXT        NOT NULL,
    min_age          SMALLINT    NOT NULL,
    max_age          SMALLINT    NOT NULL,
    duration_minutes SMALLINT    NOT NULL,
    is_published     BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT activities_age_range_chk CHECK (min_age >= 2 AND max_age <= 6 AND min_age <= max_age),
    CONSTRAINT activities_duration_chk CHECK (duration_minutes > 0)
);

CREATE INDEX IF NOT EXISTS activities_published_goal_idx ON activities (goal) WHERE is_published;

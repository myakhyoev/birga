-- Activities are soft-deleted so the completions that point at them keep their history.
ALTER TABLE activities ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

DROP INDEX IF EXISTS activities_published_goal_idx;
CREATE INDEX IF NOT EXISTS activities_published_goal_idx ON activities (goal)
    WHERE is_published AND deleted_at IS NULL;

-- One row per activity a child did on a day. completed_on is the calendar day in Uzbekistan time
-- (UTC+5), which is what streaks count. Marking the same activity twice on one day is a no-op.
CREATE TABLE IF NOT EXISTS activity_completions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    child_id     UUID        NOT NULL REFERENCES children (id) ON DELETE CASCADE,
    activity_id  UUID        NOT NULL REFERENCES activities (id) ON DELETE CASCADE,
    user_id      UUID        REFERENCES users (id) ON DELETE SET NULL,
    completed_on DATE        NOT NULL,
    note         TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT activity_completions_note_len_chk CHECK (char_length(note) <= 1000)
);

CREATE UNIQUE INDEX IF NOT EXISTS activity_completions_child_activity_day_uniq
    ON activity_completions (child_id, activity_id, completed_on);

-- Serves the streak (distinct days) and the history listing of one child.
CREATE INDEX IF NOT EXISTS activity_completions_child_day_idx
    ON activity_completions (child_id, completed_on DESC);

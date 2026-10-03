DROP TABLE IF EXISTS activity_completions;

DROP INDEX IF EXISTS activities_published_goal_idx;
CREATE INDEX IF NOT EXISTS activities_published_goal_idx ON activities (goal) WHERE is_published;

ALTER TABLE activities DROP COLUMN IF EXISTS deleted_at;

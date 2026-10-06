-- Who the user is to the children: required at sign-up, NULL for users created before it existed
-- or by an admin.
CREATE TYPE user_relationship AS ENUM ('father', 'mother', 'educator', 'nanny');

ALTER TABLE users ADD COLUMN IF NOT EXISTS relationship user_relationship;

-- Development goals a user picks at sign-up. Names are unique per language among active goals,
-- ignoring case; a soft-deleted goal frees its names.
CREATE TABLE IF NOT EXISTS goals (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name_uz    TEXT        NOT NULL,
    name_ru    TEXT        NOT NULL,
    name_en    TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS goals_name_uz_uniq ON goals (LOWER(name_uz)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS goals_name_ru_uniq ON goals (LOWER(name_ru)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS goals_name_en_uniq ON goals (LOWER(name_en)) WHERE deleted_at IS NULL;

-- A user's goals. Postgres cannot put a foreign key on array elements: the API checks the ids
-- at sign-up, and deleting a goal removes its id from every user (see goalRepo.Delete).
ALTER TABLE users ADD COLUMN IF NOT EXISTS goal_ids UUID[] NOT NULL DEFAULT '{}';

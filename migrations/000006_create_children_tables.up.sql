CREATE TABLE IF NOT EXISTS children (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT        NOT NULL,
    age        SMALLINT    NOT NULL,
    gender     TEXT        NOT NULL,
    photo_id   UUID REFERENCES media (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,

    CONSTRAINT children_age_chk CHECK (age BETWEEN 0 AND 18),
    CONSTRAINT children_gender_chk CHECK (gender IN ('male', 'female'))
);

-- Parents and children are many-to-many: a child can have several parents (caregivers) and a
-- user can have several children. Hard-deleting either side removes the link.
CREATE TABLE IF NOT EXISTS user_children (
    user_id    UUID        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    child_id   UUID        NOT NULL REFERENCES children (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (user_id, child_id)
);

-- The primary key serves lookups by user; this one serves lookups by child.
CREATE INDEX IF NOT EXISTS user_children_child_id_idx ON user_children (child_id);

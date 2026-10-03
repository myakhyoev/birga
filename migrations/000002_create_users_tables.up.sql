CREATE TABLE IF NOT EXISTS users (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT,
    username     TEXT,
    phone_number VARCHAR(15),
    photo_id     UUID,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS users_username_uniq ON users (username) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS users_phone_number_uniq ON users (phone_number) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS user_auth (
    id            UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    fcm_token     TEXT[]      NOT NULL DEFAULT '{}',
    access_token  TEXT,
    refresh_token TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Soft-deleting a user (deleted_at NULL -> NOT NULL) removes their auth row, the same as a
-- hard delete does through ON DELETE CASCADE.
CREATE OR REPLACE FUNCTION users_soft_delete_auth() RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM user_auth WHERE id = NEW.id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER users_soft_delete_auth_trg
    AFTER UPDATE OF deleted_at ON users
    FOR EACH ROW
    WHEN (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL)
    EXECUTE FUNCTION users_soft_delete_auth();

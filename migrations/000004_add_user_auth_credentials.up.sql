-- Sign-in credentials. username mirrors users.username (kept in sync by the trigger below);
-- password holds a bcrypt hash, never the plain password.
ALTER TABLE user_auth
    ADD COLUMN IF NOT EXISTS username TEXT,
    ADD COLUMN IF NOT EXISTS password TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS user_auth_username_uniq ON user_auth (username);

-- Changing users.username (for example through the admin API) renames the sign-in username too.
CREATE OR REPLACE FUNCTION users_sync_auth_username() RETURNS TRIGGER AS $$
BEGIN
    UPDATE user_auth SET username = NEW.username, updated_at = NOW() WHERE id = NEW.id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER users_sync_auth_username_trg
    AFTER UPDATE OF username ON users
    FOR EACH ROW
    WHEN (OLD.username IS DISTINCT FROM NEW.username)
    EXECUTE FUNCTION users_sync_auth_username();

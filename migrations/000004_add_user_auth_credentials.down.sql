DROP TRIGGER IF EXISTS users_sync_auth_username_trg ON users;
DROP FUNCTION IF EXISTS users_sync_auth_username();

DROP INDEX IF EXISTS user_auth_username_uniq;

ALTER TABLE user_auth
    DROP COLUMN IF EXISTS password,
    DROP COLUMN IF EXISTS username;

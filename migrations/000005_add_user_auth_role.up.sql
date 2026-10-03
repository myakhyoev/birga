-- What a user may do. Every existing row becomes a regular user.
DO $$
BEGIN
    CREATE TYPE user_role AS ENUM ('user', 'admin', 'paid_user');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END
$$;

ALTER TABLE user_auth ADD COLUMN IF NOT EXISTS role user_role NOT NULL DEFAULT 'user';

-- Postgres cannot drop an enum value: turn unverified users into regular users and rebuild the type.
UPDATE user_auth SET role = 'user' WHERE role = 'unverified_user';

ALTER TABLE user_auth ALTER COLUMN role DROP DEFAULT;
ALTER TYPE user_role RENAME TO user_role_old;
CREATE TYPE user_role AS ENUM ('user', 'admin', 'paid_user');
ALTER TABLE user_auth ALTER COLUMN role TYPE user_role USING role::text::user_role;
ALTER TABLE user_auth ALTER COLUMN role SET DEFAULT 'user';
DROP TYPE user_role_old;

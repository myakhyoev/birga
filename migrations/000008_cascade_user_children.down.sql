-- Children and links already deleted stay deleted.
DROP TRIGGER IF EXISTS users_delete_children_trg ON users;
DROP FUNCTION IF EXISTS users_delete_children();
DROP TRIGGER IF EXISTS users_soft_delete_children_trg ON users;
DROP FUNCTION IF EXISTS users_soft_delete_children();

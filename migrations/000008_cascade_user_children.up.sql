-- Deleting a user deletes their children too, unless another parent still has the child.
-- Children are many-to-many with parents (user_children), so the rule is: drop the user's links,
-- then delete every child the user was linked to that has no link left.

-- Soft delete (users.deleted_at set): remove the user's links and soft-delete the orphaned
-- children. Their activity_completions stay, like the rest of a soft-deleted child.
CREATE OR REPLACE FUNCTION users_soft_delete_children() RETURNS TRIGGER AS $$
BEGIN
    WITH unlinked AS (
        DELETE FROM user_children WHERE user_id = NEW.id RETURNING child_id
    )
    UPDATE children c
    SET deleted_at = NOW(), updated_at = NOW()
    WHERE c.id IN (SELECT child_id FROM unlinked)
      AND c.deleted_at IS NULL
      AND NOT EXISTS (SELECT 1 FROM user_children uc WHERE uc.child_id = c.id AND uc.user_id <> NEW.id);

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER users_soft_delete_children_trg
    AFTER UPDATE OF deleted_at ON users
    FOR EACH ROW
    WHEN (OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL)
    EXECUTE FUNCTION users_soft_delete_children();

-- Hard delete (DELETE FROM users): delete the children only this user has, before the row goes.
-- user_children's ON DELETE CASCADE then removes the user's remaining links, and the children's
-- completions go with them (activity_completions.child_id is ON DELETE CASCADE).
CREATE OR REPLACE FUNCTION users_delete_children() RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM children c
    WHERE c.id IN (SELECT child_id FROM user_children WHERE user_id = OLD.id)
      AND NOT EXISTS (SELECT 1 FROM user_children uc WHERE uc.child_id = c.id AND uc.user_id <> OLD.id);

    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER users_delete_children_trg
    BEFORE DELETE ON users
    FOR EACH ROW
    EXECUTE FUNCTION users_delete_children();

-- Apply the soft-delete rule to users deleted before this migration.
WITH unlinked AS (
    DELETE FROM user_children uc USING users u
    WHERE u.id = uc.user_id AND u.deleted_at IS NOT NULL
    RETURNING uc.child_id
)
UPDATE children c
SET deleted_at = NOW(), updated_at = NOW()
WHERE c.id IN (SELECT child_id FROM unlinked)
  AND c.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM user_children uc JOIN users u ON u.id = uc.user_id
      WHERE uc.child_id = c.id AND u.deleted_at IS NULL
  );

-- An account is deleted as a whole (#144): the rows that hang off it go with
-- it instead of refusing the delete. `templates` has cascaded from the start;
-- `sessions`, `files` and the file history did not, so every account that had
-- ever logged in or uploaded was impossible to delete.

ALTER TABLE files
    DROP CONSTRAINT files_user_id_fkey,
    ADD CONSTRAINT files_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE files_history
    DROP CONSTRAINT files_history_file_id_fkey,
    ADD CONSTRAINT files_history_file_id_fkey
        FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE;

ALTER TABLE sessions
    DROP CONSTRAINT sessions_user_id_fkey,
    ADD CONSTRAINT sessions_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- `commands` never had a foreign key at all, so an account deleted before this
-- change left its command library behind with nothing pointing at the account.
-- Those rows are cleaned here — only rows no account owns, which is what an
-- orphan is — and then the schema holds the rule the same way it does for
-- `templates`.
--
-- A command an account owns can still be refused by `template_commands`
-- (`ON DELETE RESTRICT`) when another account's template pulls it in: that is
-- the deliberate rule, and the API answers 409 for it rather than deleting a
-- script out of somebody else's published template.
DELETE FROM template_commands
 WHERE command_id IN (
    SELECT id FROM commands WHERE user_id NOT IN (SELECT id FROM users)
 );

DELETE FROM commands
 WHERE user_id NOT IN (SELECT id FROM users);

-- The rows are gone; their binaries under `<upload-root>/commands/<command-id>/`
-- are not. A migration cannot know the upload root, so those folders are left
-- for an operator to remove.

ALTER TABLE commands
    ADD CONSTRAINT commands_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

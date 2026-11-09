-- Back to the constraints #144 found: the dependents refuse the delete again.
--
-- This is not a full undo: the orphan commands the up migration deleted — and
-- the folder bytes they left — cannot be restored.

ALTER TABLE commands
    DROP CONSTRAINT commands_user_id_fkey;

ALTER TABLE sessions
    DROP CONSTRAINT sessions_user_id_fkey,
    ADD CONSTRAINT sessions_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE files_history
    DROP CONSTRAINT files_history_file_id_fkey,
    ADD CONSTRAINT files_history_file_id_fkey
        FOREIGN KEY (file_id) REFERENCES files(id);

ALTER TABLE files
    DROP CONSTRAINT files_user_id_fkey,
    ADD CONSTRAINT files_user_id_fkey
        FOREIGN KEY (user_id) REFERENCES users(id);

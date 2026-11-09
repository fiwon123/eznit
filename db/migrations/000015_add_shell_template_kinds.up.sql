-- A template can target the shell a developer uses, not only an AI terminal.
-- The starter for `bash`, `zsh`, `fish` and `powershell` writes a shell file;
-- a template may carry those kinds like any other.
--
-- The set is enforced here as well as in the service: a row cannot hold a
-- kind the code does not know, whatever path wrote it.
ALTER TABLE templates DROP CONSTRAINT templates_kind_check;

ALTER TABLE templates ADD CONSTRAINT templates_kind_check
    CHECK (kind IN (
        'opencode', 'codex', 'copilot', 'cursor', 'windsurf', 'generic',
        'bash', 'zsh', 'fish', 'powershell'
    ));

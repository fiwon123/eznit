-- A shell kind is not something the old schema knows, and the old CHECK would
-- refuse any row that holds one. A down migration cannot keep a value the old
-- schema rejects, so those rows move to `generic` before the constraint goes
-- back: the template is still readable, and the shell it targeted is what the
-- new build wrote, not something the old one can represent.
UPDATE templates
   SET kind = 'generic'
 WHERE kind IN ('bash', 'zsh', 'fish', 'powershell');

ALTER TABLE templates DROP CONSTRAINT templates_kind_check;

ALTER TABLE templates ADD CONSTRAINT templates_kind_check
    CHECK (kind IN ('opencode', 'codex', 'copilot', 'cursor', 'windsurf', 'generic'));

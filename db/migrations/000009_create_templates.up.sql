-- A template is a named, versioned folder structure a user can publish and
-- others can apply. The files live in the database (they are configuration
-- files, small and text), so the catalogue can be searched and the whole
-- structure travels in one row set.
--
-- The catalogue is owner-scoped from the start: `is_public` decides whether a
-- template is in the shared catalogue, and every read is filtered by the
-- caller. The files domain lists every upload in the instance to every user;
-- that is not repeated here.

CREATE TABLE templates (
    id          UUID PRIMARY KEY NOT NULL,
    user_id     UUID NOT NULL,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    -- The AI terminal the template targets. The set is enforced here as well
    -- as in the service: a constraint in Postgres cannot be forgotten by a new
    -- code path, a check in Go can.
    kind        TEXT NOT NULL DEFAULT 'generic'
                CHECK (kind IN ('opencode', 'codex', 'copilot', 'cursor', 'windsurf', 'generic')),
    version     INTEGER NOT NULL DEFAULT 1,
    is_public   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE template_files (
    id            UUID PRIMARY KEY NOT NULL,
    template_id   UUID NOT NULL,
    -- The path inside the template, relative to the project root:
    -- `.opencode/agent.md`. It is unique per template.
    path          TEXT NOT NULL,
    content       TEXT NOT NULL DEFAULT '',
    is_executable BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (template_id) REFERENCES templates(id) ON DELETE CASCADE,
    UNIQUE (template_id, path)
);

CREATE TABLE template_variables (
    id            UUID PRIMARY KEY NOT NULL,
    template_id   UUID NOT NULL,
    -- The name substituted in the file contents: `{{PROJECT_NAME}}`.
    -- The column is `var_key` and not `key`: KEY is a keyword in Postgres,
    -- and a column name that has to be quoted everywhere is a trap.
    var_key       TEXT NOT NULL,
    prompt        TEXT NOT NULL DEFAULT '',
    default_value TEXT NOT NULL DEFAULT '',
    required      BOOLEAN NOT NULL DEFAULT FALSE,
    sort_order    INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (template_id) REFERENCES templates(id) ON DELETE CASCADE,
    UNIQUE (template_id, var_key)
);

CREATE INDEX idx_templates_user_id ON templates (user_id);
CREATE INDEX idx_template_files_template_id ON template_files (template_id);
CREATE INDEX idx_template_variables_template_id ON template_variables (template_id);

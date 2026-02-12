CREATE TABLE files (
    id  BIGSERIAL PRIMARY KEY,
    id_user BIGSERIAL NOT NULL,
    name TEXT,
    ext TEXT,
    path TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (id) REFERENCES users(id)
);

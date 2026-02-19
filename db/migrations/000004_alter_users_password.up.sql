ALTER TABLE users
ALTER COLUMN password TYPE BYTEA
USING password::bytea;

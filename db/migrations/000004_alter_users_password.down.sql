ALTER TABLE users
ALTER COLUMN password TYPE TEXT
USING convert_from(password, 'UTF8');

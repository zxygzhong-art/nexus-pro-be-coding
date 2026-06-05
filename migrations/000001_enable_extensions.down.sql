-- Note: app_user may own grants on objects dropped by later down-migrations; drop last.
DROP ROLE IF EXISTS app_user;
DROP EXTENSION IF EXISTS pgcrypto;

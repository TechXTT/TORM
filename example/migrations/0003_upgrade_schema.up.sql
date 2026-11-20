-- Backfill usernames from stable IDs and retain the legacy name columns.
ALTER TABLE creator ADD COLUMN username TEXT;
UPDATE creator SET username = 'creator_' || replace(id::text, '-', '');
UPDATE creator SET createdat = now() WHERE createdat IS NULL;
UPDATE creator SET updatedat = createdat WHERE updatedat IS NULL;
UPDATE post SET createdat = now() WHERE createdat IS NULL;
UPDATE post SET updatedat = createdat WHERE updatedat IS NULL;

-- Required values, duplicate emails, and invalid or orphaned author IDs must
-- be repaired explicitly before retrying this migration.
ALTER TABLE creator
    ALTER COLUMN id DROP DEFAULT,
    ALTER COLUMN username SET NOT NULL,
    ALTER COLUMN email SET NOT NULL,
    ALTER COLUMN createdat TYPE TIMESTAMP(6),
    ALTER COLUMN createdat SET DEFAULT CURRENT_TIMESTAMP,
    ALTER COLUMN createdat SET NOT NULL,
    ALTER COLUMN updatedat TYPE TIMESTAMP(6),
    ALTER COLUMN updatedat DROP DEFAULT,
    ALTER COLUMN updatedat SET NOT NULL,
    ADD CONSTRAINT creator_username_key UNIQUE (username),
    ADD CONSTRAINT creator_email_key UNIQUE (email);
ALTER TABLE post
    ALTER COLUMN id DROP DEFAULT,
    ALTER COLUMN title SET NOT NULL,
    ALTER COLUMN content SET NOT NULL,
    ALTER COLUMN published SET NOT NULL,
    ALTER COLUMN createdat TYPE TIMESTAMP(6),
    ALTER COLUMN createdat SET DEFAULT CURRENT_TIMESTAMP,
    ALTER COLUMN createdat SET NOT NULL,
    ALTER COLUMN updatedat TYPE TIMESTAMP(6),
    ALTER COLUMN updatedat DROP DEFAULT,
    ALTER COLUMN updatedat SET NOT NULL,
    ALTER COLUMN authorid TYPE UUID USING authorid::uuid,
    ALTER COLUMN authorid SET NOT NULL,
    ADD CONSTRAINT post_authorid_fkey FOREIGN KEY (authorid)
        REFERENCES creator (id) ON DELETE RESTRICT ON UPDATE CASCADE;
CREATE INDEX post_authorid_idx ON post (authorid);

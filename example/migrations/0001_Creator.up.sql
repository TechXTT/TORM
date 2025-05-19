CREATE TABLE creator (
    id TEXT PRIMARY KEY,
    firstname TEXT NOT NULL,
    lastname TEXT NOT NULL,
    email TEXT NOT NULL,
    createdat TIMESTAMP DEFAULT now(),
    updatedat TIMESTAMP NOT NULL,
    posts TEXT NOT NULL
);
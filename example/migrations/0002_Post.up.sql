CREATE TABLE post (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    published BOOLEAN DEFAULT false,
    createdat TIMESTAMP DEFAULT now(),
    updatedat TIMESTAMP NOT NULL,
    authorid TEXT NOT NULL,
    author TEXT NOT NULL
);
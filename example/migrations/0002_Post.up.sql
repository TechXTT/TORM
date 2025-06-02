CREATE TABLE post (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    title TEXT,
    content TEXT,
    published BOOLEAN DEFAULT false,
    createdat TIMESTAMP DEFAULT now(),
    updatedat TIMESTAMP DEFAULT now(),
    authorid TEXT
);
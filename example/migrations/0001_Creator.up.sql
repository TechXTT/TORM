CREATE TABLE creator (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    firstname TEXT,
    lastname TEXT,
    email TEXT,
    createdat TIMESTAMP DEFAULT now(),
    updatedat TIMESTAMP DEFAULT now()
);
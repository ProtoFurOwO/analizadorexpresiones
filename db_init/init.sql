CREATE TABLE historial_consultas (
    id SERIAL PRIMARY KEY,
    query_text TEXT NOT NULL,
    is_valid BOOLEAN NOT NULL,
    matches_count INTEGER DEFAULT 0,
    fecha TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
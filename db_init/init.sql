CREATE TABLE historial_consultas (
    id SERIAL PRIMARY KEY,
    query_text TEXT NOT NULL,
    is_valid BOOLEAN NOT NULL,
    fecha TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
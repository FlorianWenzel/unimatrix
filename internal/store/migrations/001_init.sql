CREATE TABLE drones (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    designation   TEXT    NOT NULL UNIQUE,
    password_hash TEXT    NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE transmissions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    drone_id   INTEGER NOT NULL REFERENCES drones(id) ON DELETE CASCADE,
    body       TEXT    NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_transmissions_created_at ON transmissions(created_at DESC);
CREATE INDEX idx_transmissions_drone_id   ON transmissions(drone_id);

CREATE TABLE likes (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    transmission_id INTEGER NOT NULL REFERENCES transmissions(id) ON DELETE CASCADE,
    drone_id     INTEGER NOT NULL REFERENCES drones(id) ON DELETE CASCADE,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(transmission_id, drone_id)
);

CREATE INDEX idx_likes_transmission_id ON likes(transmission_id);
CREATE INDEX idx_likes_drone_id ON likes(drone_id);
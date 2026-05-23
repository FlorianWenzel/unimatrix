CREATE TABLE follows(
    follower_id INTEGER NOT NULL REFERENCES drones(id),
    followee_id INTEGER NOT NULL REFERENCES drones(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (follower_id, followee_id)
);

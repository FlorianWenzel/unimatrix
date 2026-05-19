-- Add last_seen timestamp to track drone activity
ALTER TABLE drones ADD COLUMN last_seen TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE photo_scores DROP COLUMN is_complete;
ALTER TABLE photo_scores ADD COLUMN is_complete BOOLEAN GENERATED ALWAYS AS (
    road_within_1000m OR validity_score is not null
    ) STORED;

ALTER TABLE photo_scores DROP COLUMN gps_altitude;
ALTER TABLE photo_scores DROP COLUMN gps_altitude_available;
ALTER TABLE photo_scores DROP COLUMN terrain_altitude;

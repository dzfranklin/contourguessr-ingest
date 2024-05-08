ALTER TABLE photo_scores ADD COLUMN gps_altitude FLOAT;
ALTER TABLE photo_scores ADD COLUMN gps_altitude_available BOOLEAN;
ALTER TABLE photo_scores ADD COLUMN terrain_altitude FLOAT;

ALTER TABLE photo_scores DROP COLUMN is_complete;
ALTER TABLE photo_scores ADD COLUMN is_complete BOOLEAN GENERATED ALWAYS AS (
    road_within_1000m OR
    validity_score < 0.5 OR
    not gps_altitude_available OR
    (gps_altitude is not null and terrain_altitude is not null)
) STORED;

-- Note we leave is_accepted alone for now

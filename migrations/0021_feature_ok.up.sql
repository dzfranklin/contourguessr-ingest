ALTER TABLE features
    ADD COLUMN is_ok boolean GENERATED ALWAYS AS (
        validity_score > 0.5
            AND nearest_road_meters >= 75
            AND (no_gps_altitude OR (gps_altitude_meters - terrain_elevation_meters) < 1000)
        ) STORED;

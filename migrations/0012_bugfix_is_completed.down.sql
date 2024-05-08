ALTER TABLE photo_scores
    DROP COLUMN is_complete;
ALTER TABLE photo_scores
    ADD COLUMN is_complete boolean generated always as ((road_within_1000m OR
                                                         (validity_score < (0.5)::double precision) OR
                                                         (NOT gps_altitude_available) OR
                                                         ((gps_altitude IS NOT NULL) AND (terrain_altitude IS NOT NULL)))) stored;

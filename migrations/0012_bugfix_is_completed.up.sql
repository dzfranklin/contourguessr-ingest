ALTER TABLE photo_scores
    DROP COLUMN is_complete;
ALTER TABLE photo_scores
    ADD COLUMN is_complete BOOLEAN GENERATED ALWAYS AS ((road_within_1000m OR
                                                         (validity_score is not null AND validity_score < 0.5) OR
                                                         (gps_altitude_available is not null AND not gps_altitude_available) OR
                                                         ((gps_altitude IS NOT NULL) AND (terrain_altitude IS NOT NULL)))) STORED;

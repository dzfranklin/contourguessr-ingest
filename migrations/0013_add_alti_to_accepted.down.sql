ALTER TABLE photo_scores
    DROP COLUMN is_accepted;

ALTER TABLE photo_scores
    ADD COLUMN is_accepted BOOLEAN GENERATED ALWAYS AS ((NOT road_within_1000m) AND
                                                        (validity_score >= 0.5)) STORED;

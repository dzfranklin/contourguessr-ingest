ALTER TABLE photo_scores ADD COLUMN is_complete BOOLEAN GENERATED ALWAYS AS (
    road_within_1000m OR validity_score is not null
) STORED;

ALTER TABLE photo_scores ADD COLUMN is_accepted BOOLEAN GENERATED ALWAYS AS (
    not road_within_1000m AND validity_score >= 0.5
) STORED;

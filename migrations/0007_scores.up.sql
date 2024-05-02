DROP TABLE IF EXISTS photo_validity_scores;

CREATE TABLE photo_scores (
    id BIGSERIAL PRIMARY KEY,
    flickr_photo_id TEXT REFERENCES flickr_photos(flickr_id),
    updated_at TIMESTAMP,

    validity_score FLOAT,
    validity_model TEXT,

    road_within_1000m BOOLEAN
)

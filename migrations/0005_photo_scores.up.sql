CREATE TABLE photo_validity_scores
(
    id             BIGSERIAL PRIMARY KEY,
    flickr_id      TEXT REFERENCES flickr_photos (flickr_id),
    validity_score FLOAT,
    validity_model TEXT,
    inserted_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE flickr_photo_fetch_failures
(
    id        BIGSERIAL PRIMARY KEY,
    flickr_id TEXT,
    err       TEXT,
    inserted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
)

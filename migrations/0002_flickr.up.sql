CREATE TABLE flickr_photos
(
    flickr_id   TEXT PRIMARY KEY,
    summary     jsonb,
    info        jsonb,
    sizes       jsonb,
    exif        jsonb,
    raw_exif    jsonb,
    geo       GEOGRAPHY(Point, 4326),
    geo_accuracy INT,
    inserted_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE flickr_indexer_progress
(
    region_id   INT PRIMARY KEY REFERENCES regions (id) ON DELETE CASCADE,
    latest_seen TIMESTAMP
)

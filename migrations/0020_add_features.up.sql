CREATE TABLE features (
    photo_id TEXT PRIMARY KEY REFERENCES flickr(id),
    terrain_elevation_meters INT,
    no_gps_altitude bool DEFAULT false,
    gps_altitude_meters INT,
    validity_score FLOAT,
    validity_model TEXT,
    nearest_road_meters INT
)

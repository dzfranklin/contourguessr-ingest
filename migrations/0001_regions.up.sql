CREATE TABLE regions
(
    id           SERIAL PRIMARY KEY,
    geo          GEOGRAPHY(POLYGON, 4326),
    name         TEXT,
    country_iso2 TEXT,
    logo_url     TEXT,
    active       BOOLEAN DEFAULT FALSE NOT NULL
)

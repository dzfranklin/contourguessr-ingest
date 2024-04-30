CREATE EXTENSION IF NOT EXISTS postgis WITH SCHEMA public;

CREATE TABLE flickr_photos
(
    id            text NOT NULL,
    region        text,
    owner         text,
    shared_secret text,
    server        text,
    date_taken    timestamp without time zone,
    geom          geometry(Point, 4326),
    geom_accuracy double precision,
    summary       jsonb,
    info          jsonb,
    sizes         jsonb,
    exif          jsonb,
    raw_exif      jsonb,
    inserted_at   timestamp without time zone DEFAULT now(),
    gps_altitude  double precision GENERATED ALWAYS AS (
                      CASE
                          WHEN ((exif ->> 'GPSAltitude'::text) ~~ '% m'::text)
                              THEN (TRIM(BOTH ' m'::text FROM (exif ->> 'GPSAltitude'::text)))::double precision
                          ELSE NULL::double precision
                          END) STORED,
    web_url       text GENERATED ALWAYS AS (((('https://flickr.com/photos/'::text || owner) || '/'::text) || id)) STORED,
    medium_src    text GENERATED ALWAYS AS ((
        ((((('https://live.staticflickr.com/'::text || server) || '/'::text) || id) || '_'::text) || shared_secret) ||
        '.jpg'::text)) STORED,
    small_src     text GENERATED ALWAYS AS ((
        ((((('https://live.staticflickr.com/'::text || server) || '/'::text) || id) || '_'::text) || shared_secret) ||
        '_m.jpg'::text)) STORED
);

CREATE TABLE labels
(
    id              integer                                               NOT NULL,
    flickr_photo_id text                                                  NOT NULL,
    is_positive     boolean,
    created_at      timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);

CREATE TABLE offroad_flickr_photos
(
    id text NOT NULL
);

CREATE TABLE roads
(
    id        integer NOT NULL,
    osm_id    text,
    line_geom geography
);

CREATE INDEX roads_line_geom_idx ON roads USING gist (line_geom);

CREATE TABLE temp_labelled_photos
(
    web_url     text,
    is_positive boolean,
    geom        geometry(Point, 4326)
);

CREATE table challenges
(
    id                bigserial primary key,
    region_id         int references regions (id),

    geo               geography(Point, 4326),

    preview_src       text,
    preview_width     int,
    preview_height    int,

    regular_src       text,
    regular_width     int,
    regular_height    int,

    large_src         text,
    large_width       int,
    large_height      int,

    photographer_icon text,
    photographer_text text,
    photographer_link text,

    title             text,
    description_html  text,
    date_taken        timestamp,
    link              text,

    rx                float,
    ry                float
);

CREATE table flickr_challenge_sources
(
    challenge_id bigint references challenges (id) on delete cascade,
    flickr_id    text references flickr_photos (flickr_id),
    primary key (challenge_id, flickr_id)
)

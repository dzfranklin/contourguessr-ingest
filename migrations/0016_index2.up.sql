create table flickr
(
    id          text not null primary key,
    region_id   integer references regions on delete cascade,
    inserted_at timestamp default current_timestamp,
    info        jsonb,
    sizes       jsonb,
    exif        jsonb,

    medium     jsonb,
    large      jsonb
);

create table region_index_cursors
(
    region_id       integer primary key references regions on delete cascade,
    min_upload_date timestamp,
    page            integer,
    last_check timestamp
);

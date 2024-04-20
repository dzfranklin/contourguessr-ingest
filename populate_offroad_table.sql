DROP TABLE IF EXISTS offroad_flickr_photos;

CREATE TABLE offroad_flickr_photos
(
    id TEXT PRIMARY KEY REFERENCES flickr_photos (id) ON DELETE CASCADE
);

INSERT INTO offroad_flickr_photos
SELECT p.id
FROM flickr_photos p
         LEFT JOIN roads as r
                   ON ST_DWithin(p.geom, r.line_geom, 400)
WHERE r IS NULL
  AND (p.exif IS NULL OR p.exif->>'Software' NOT ILIKE '%RoboGEO%') -- Offers ability to watermark with geocoding
;

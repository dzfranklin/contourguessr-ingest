SELECT *
FROM regions;

SELECT r.id, r.name, count(p) as count, max(fip.latest_seen) as latest_seen
FROM flickr_photos as p
         RIGHT JOIN regions as r ON p.region_id = r.id
         LEFT JOIN flickr_indexer_progress fip on r.id = fip.region_id
GROUP BY r.id
ORDER BY count DESC;

SELECT count(*)
FROM flickr_photos;

SELECT * FROM flickr_photos ORDER BY random() LIMIT 10;

SELECT 'https://flickr.com/' || (summary ->> 'owner') || '/' || (summary ->> 'id')
FROM flickr_photos
ORDER BY random()
LIMIT 10;

---

-- Count by month by region
SELECT r.name, (date_part('month', (p.summary->>'datetaken')::timestamp)) as month, count(*)
FROM flickr_photos as p
         RIGHT JOIN regions as r ON p.region_id = r.id
WHERE p.summary->>'datetaken' != '0000-00-00 00:00:00'
GROUP BY r.id, month
ORDER BY r.id, month;

---

-- Almost all photos have an accuracy of 16. Maybe we should disregard 15?

SELECT geo_accuracy, count(*)
FROM flickr_photos
GROUP BY geo_accuracy
ORDER BY geo_accuracy DESC;

---

-- GOAL: Display a heatmap that gives a visual representation of where photos are in a region
--   It should be detailed enough I can see things like near road vs backcountry, but few enough
--   points to practically map. Let's try rounding all coordinates to 1 arc-second (0.0002777 degrees)

SELECT count(DISTINCT
             (round(ST_Y(geo::geometry)::numeric, 2),
              round(ST_X(geo::geometry)::numeric, 2))) as trunc_count,
    count(*) as total
FROM flickr_photos;

SELECT
    round(ST_X(geo::geometry)::numeric, 2) as lon,
    round(ST_Y(geo::geometry)::numeric, 2) as lat,
    count(*) as count
FROM flickr_photos
GROUP BY lat, lon;

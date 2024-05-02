SELECT *
FROM regions;

SELECT r.id, r.name, count(p) as count, fip.latest_request
FROM flickr_photos as p
         RIGHT JOIN regions as r ON p.region_id = r.id
         LEFT JOIN flickr_indexer_progress fip on r.id = fip.region_id
GROUP BY r.id, fip.latest_request
ORDER BY fip.latest_request DESC;

SELECT count(*)
FROM flickr_photos;

SELECT *
FROM flickr_photos
ORDER BY random()
LIMIT 10;

SELECT 'https://flickr.com/' || (summary ->> 'owner') || '/' || (summary ->> 'id')
FROM flickr_photos
ORDER BY random()
LIMIT 10;

---

-- Count by month by region
SELECT r.name, (date_part('month', (p.summary ->> 'datetaken')::timestamp)) as month, count(*)
FROM flickr_photos as p
         RIGHT JOIN regions as r ON p.region_id = r.id
WHERE p.summary ->> 'datetaken' != '0000-00-00 00:00:00'
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
       count(*)                                        as total
FROM flickr_photos;

SELECT round(ST_X(geo::geometry)::numeric, 2) as lon,
       round(ST_Y(geo::geometry)::numeric, 2) as lat,
       count(*)                               as count
FROM flickr_photos
GROUP BY lat, lon;

---

SELECT 'https://flickr.com/' || (p.summary ->> 'owner') || '/' || p.flickr_id,
       s.road_within_1000m,
       s.validity_score,
       s.validity_model,
       s.updated_at
FROM photo_scores as s
         LEFT JOIN flickr_photos as p ON s.flickr_photo_id = p.flickr_id
ORDER BY updated_at DESC;

SELECT count(*)                                                                                            as total,
       count(*) filter (where validity_score > 0.5 and not road_within_1000m)                              as accept,
       round(100.0 * count(*) filter (where validity_score > 0.5 and not road_within_1000m) / count(*), 2) as pc_accept,
       count(*) filter (where not road_within_1000m)                                                       as no_road,
       round(100.0 * count(*) filter (where not road_within_1000m) / count(*),
             2)                                                                                            as pc_no_road,
       count(*) filter (where validity_score > 0.5)                                                        as valid,
       round(100.0 * count(*) filter (where validity_score > 0.5) / (count(*)), 2)                         as pc_valid,
       count(*) filter (where validity_score is not null)                                                  as validity_scored
FROM photo_scores;

SELECT r.name,
       count(s.id)                                                   as total,
       count(s.id)
       filter (where validity_score > 0.5 and not road_within_1000m) as accept,
       round(100.0 * count(s.id) filter (where validity_score > 0.5 and not road_within_1000m) / count(s.id),
             2)                                                      as pc_accept,
       count(s.id) filter (where not road_within_1000m)              as no_road,
       round(100.0 * count(*) filter (where not road_within_1000m) / count(s.id),
             2)                                                      as pc_no_road,
       count(s.id) filter (where validity_score > 0.5)               as valid,
       round(100.0 * count(s.id) filter (where validity_score > 0.5) / (count(s.id)),
             2)                                                      as pc_valid,
       count(s.id) filter (where validity_score is not null)            as validity_scored
FROM photo_scores as s
         LEFT JOIN flickr_photos as p ON s.flickr_photo_id = p.flickr_id
         LEFT JOIN regions as r ON p.region_id = r.id
GROUP BY r.id
ORDER BY pc_accept DESC;

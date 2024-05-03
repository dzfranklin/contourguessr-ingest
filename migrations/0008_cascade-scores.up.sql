-- alter photo_scores_flickr_photo_id_fkey to on delete cascade
ALTER TABLE photo_scores
    DROP CONSTRAINT photo_scores_flickr_photo_id_fkey;
ALTER TABLE photo_scores
    ADD CONSTRAINT photo_scores_flickr_photo_id_fkey FOREIGN KEY (flickr_photo_id)
        REFERENCES flickr_photos (flickr_id) ON DELETE CASCADE;

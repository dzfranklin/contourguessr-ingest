ALTER TABLE photo_scores ADD COLUMN vsn INT DEFAULT 1 NOT NULL;
ALTER TABLE photo_scores ALTER COLUMN vsn DROP DEFAULT;

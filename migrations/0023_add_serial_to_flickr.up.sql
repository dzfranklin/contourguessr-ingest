ALTER TABLE flickr ADD COLUMN serial bigserial;
CREATE INDEX flickr_serial_idx ON flickr (serial);

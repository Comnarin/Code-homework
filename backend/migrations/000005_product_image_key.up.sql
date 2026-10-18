-- Name of the product's picture in object storage (MinIO). NULL = no picture yet.
ALTER TABLE products ADD COLUMN image_key text;

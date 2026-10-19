DROP INDEX IF EXISTS order_items_limit_anchor_idx;
ALTER TABLE order_items DROP COLUMN IF EXISTS started_limit_window;
ALTER TABLE orders DROP COLUMN IF EXISTS client_key;

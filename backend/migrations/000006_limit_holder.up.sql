-- Postgres becomes the authoritative record of who holds a limited product.
-- orders.client_key is the holder key (NULL = anonymous, the order id is the holder);
-- order_items.started_limit_window marks the order that opened a limit window,
-- so a same-holder re-price can never extend it.
ALTER TABLE orders ADD COLUMN client_key text;
ALTER TABLE order_items ADD COLUMN started_limit_window boolean NOT NULL DEFAULT false;
CREATE INDEX order_items_limit_anchor_idx ON order_items (product_id) WHERE started_limit_window;

-- Orders still inside an active window were the ones that opened it.
UPDATE order_items i
SET started_limit_window = true
FROM orders o, product_limits pl
WHERE o.id = i.order_id
  AND pl.product_id = i.product_id
  AND pl.active
  AND o.created_at > now() - make_interval(secs => pl.window_seconds);

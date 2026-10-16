CREATE TABLE products (
    id           text PRIMARY KEY,
    name         text NOT NULL,
    price_satang integer NOT NULL CHECK (price_satang > 0),
    sort_order   integer NOT NULL
);

CREATE TABLE members (
    card_number text PRIMARY KEY,
    full_name   text NOT NULL,
    status      text NOT NULL CHECK (status IN ('ACTIVE', 'SUSPENDED')),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE rules (
    id          serial PRIMARY KEY,
    code        text NOT NULL UNIQUE,
    type        text NOT NULL CHECK (type IN ('BUNDLE_PERCENT', 'MEMBER_PERCENT')),
    percent_bp  integer NOT NULL CHECK (percent_bp BETWEEN 1 AND 10000),
    bundle_size integer,
    priority    integer NOT NULL,
    active      boolean NOT NULL DEFAULT true,
    CHECK ((type = 'BUNDLE_PERCENT') = (bundle_size IS NOT NULL))
);

CREATE TABLE rule_products (
    rule_id    integer NOT NULL REFERENCES rules (id) ON DELETE CASCADE,
    product_id text NOT NULL REFERENCES products (id),
    PRIMARY KEY (rule_id, product_id)
);

CREATE TABLE product_limits (
    product_id     text PRIMARY KEY REFERENCES products (id),
    window_seconds integer NOT NULL CHECK (window_seconds > 0),
    active         boolean NOT NULL DEFAULT true
);

CREATE TABLE orders (
    id                     uuid PRIMARY KEY,
    customer_id            text NOT NULL,
    member_card            text REFERENCES members (card_number),
    total_before_satang    integer NOT NULL,
    member_discount_satang integer NOT NULL,
    final_total_satang     integer NOT NULL,
    created_at             timestamptz NOT NULL
);

CREATE TABLE order_items (
    order_id   uuid NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    product_id text NOT NULL REFERENCES products (id),
    quantity   integer NOT NULL CHECK (quantity > 0),
    PRIMARY KEY (order_id, product_id)
);

CREATE TABLE order_discounts (
    id            serial PRIMARY KEY,
    order_id      uuid NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    rule_code     text NOT NULL,
    product_id    text REFERENCES products (id),
    amount_satang integer NOT NULL
);

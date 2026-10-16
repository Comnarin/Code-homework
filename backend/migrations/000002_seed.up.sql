INSERT INTO products (id, name, price_satang, sort_order) VALUES
    ('red',    'Red set',    5000, 1),
    ('green',  'Green set',  4000, 2),
    ('blue',   'Blue set',   3000, 3),
    ('yellow', 'Yellow set', 5000, 4),
    ('pink',   'Pink set',   8000, 5),
    ('purple', 'Purple set', 9000, 6),
    ('orange', 'Orange set', 12000, 7);

INSERT INTO members (card_number, full_name, status) VALUES
    ('MEM0001', 'Demo Member', 'ACTIVE'),
    ('MEM0002', 'Suspended Member', 'SUSPENDED');

INSERT INTO rules (code, type, percent_bp, bundle_size, priority) VALUES
    ('PAIR_5',    'BUNDLE_PERCENT', 500,  2,    10),
    ('MEMBER_10', 'MEMBER_PERCENT', 1000, NULL, 100);

INSERT INTO rule_products (rule_id, product_id)
SELECT r.id, p FROM rules r, unnest(ARRAY['orange', 'pink', 'green']) AS p
WHERE r.code = 'PAIR_5';

INSERT INTO product_limits (product_id, window_seconds) VALUES ('red', 3600);

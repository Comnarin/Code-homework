-- Orders are not tied to a customer: the Red-set rule is "ordered in the last hour", for everyone.
ALTER TABLE orders DROP COLUMN customer_id;

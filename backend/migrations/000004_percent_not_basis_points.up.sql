-- Discount percentages are stored as whole percents (5 = 5%), not basis points (500).
ALTER TABLE rules DROP CONSTRAINT rules_percent_bp_check;
UPDATE rules SET percent_bp = percent_bp / 100;
ALTER TABLE rules RENAME COLUMN percent_bp TO percent;
ALTER TABLE rules ADD CONSTRAINT rules_percent_check CHECK (percent BETWEEN 1 AND 100);

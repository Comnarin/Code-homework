ALTER TABLE rules DROP CONSTRAINT rules_percent_check;
ALTER TABLE rules RENAME COLUMN percent TO percent_bp;
UPDATE rules SET percent_bp = percent_bp * 100;
ALTER TABLE rules ADD CONSTRAINT rules_percent_bp_check CHECK (percent_bp BETWEEN 1 AND 10000);

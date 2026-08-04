ALTER TABLE queue_items ADD COLUMN is_default INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1));
CREATE UNIQUE INDEX queue_items_single_default ON queue_items(is_default) WHERE is_default = 1;

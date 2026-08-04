ALTER TABLE queue_items ADD COLUMN title TEXT NOT NULL DEFAULT '';

UPDATE queue_items
SET title = (SELECT title FROM queue_state WHERE singleton = 1)
WHERE id = (SELECT current_item_id FROM queue_state WHERE singleton = 1);

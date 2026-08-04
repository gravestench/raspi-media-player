DELETE FROM queue_items WHERE is_default = 1;

UPDATE queue_state
SET current_item_id = NULL,
    playback_status = 'idle',
    title = '',
    position_seconds = 0,
    duration_seconds = 0,
    paused = 0,
    buffering = 0,
    playback_error = '',
    updated_at = CURRENT_TIMESTAMP
WHERE current_item_id IS NOT NULL
  AND current_item_id NOT IN (SELECT id FROM queue_items);

WITH ordered AS (
    SELECT id, ROW_NUMBER() OVER (ORDER BY position) - 1 AS new_position
    FROM queue_items
)
UPDATE queue_items
SET position = (SELECT new_position FROM ordered WHERE ordered.id = queue_items.id);

DELETE FROM application_settings
WHERE key IN ('default_radio_url', 'default_radio_name');

DROP INDEX queue_items_single_default;
ALTER TABLE queue_items DROP COLUMN is_default;

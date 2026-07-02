-- name: UpdateVideoStatus :exec
UPDATE video
SET status = $1,
    updated_at = now()
WHERE id = $2;

-- name: UpdateVideoProgress :exec
UPDATE video
SET progress = $1,
    updated_at = now()
WHERE id = $2;

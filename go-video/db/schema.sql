-- Mirrors the `video` table from the Next.js app (app/src/db/schema.ts).
-- Only the columns the renderer touches are needed, but the full shape is kept
-- so sqlc models match the real table.
--
-- `progress` is new here and does NOT yet exist on the Next.js app's schema —
-- it's additive (0-100, default 0) so it's safe to add without a backfill,
-- but the app's schema.ts + a drizzle migration need the same column before
-- this is deployed against the shared production DB.
CREATE TABLE video (
    id          uuid PRIMARY KEY,
    user_id     text NOT NULL,
    type        text NOT NULL,
    credit_cost integer,
    status      text NOT NULL DEFAULT 'queued',
    progress    integer NOT NULL DEFAULT 0,
    created_at  timestamp NOT NULL DEFAULT now(),
    updated_at  timestamp NOT NULL DEFAULT now()
);

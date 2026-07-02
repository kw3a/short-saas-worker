import { Pool } from "pg";

// Talks directly to the same Postgres go-video's internal/store package writes
// to (see ../../go-video/db/schema.sql). We only ever touch the `video` table's
// status tracking columns here — no credits/users/balance tables, since those
// belong to the Next.js app and this client intentionally bypasses it.
const pool = new Pool({ connectionString: import.meta.env.DATABASE_URL });

const TEST_CLIENT_USER_ID = "video-test-client";

export async function createStubVideoRow(id: string, type: "narration" | "askreddit") {
  await pool.query(
    `INSERT INTO video (id, user_id, type, status) VALUES ($1, $2, $3, 'queued')
     ON CONFLICT (id) DO NOTHING`,
    [id, TEST_CLIENT_USER_ID, type]
  );
}

export type VideoState = { status: string; progress: number };

export async function getVideoState(id: string): Promise<VideoState | null> {
  const { rows } = await pool.query<VideoState>(
    `SELECT status, progress FROM video WHERE id = $1`,
    [id]
  );
  return rows[0] ?? null;
}

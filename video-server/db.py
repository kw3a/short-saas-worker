import os
import psycopg2
from dotenv import load_dotenv

def _db_conn():
    # Use DATABASE_URL like: postgres://user:pass@host:port/dbname
    dsn = os.environ["DATABASE_URL"]
    conn = psycopg2.connect(dsn)
    conn.autocommit = True
    return conn

def _db_update_video_status(job_id: str, status: str):
    try:
        with _db_conn() as conn:
            with conn.cursor() as cur:
                cur.execute("UPDATE video SET status = %s WHERE id = %s", (status, job_id))
    except Exception as e:
        print(f"DB status update failed for {job_id} -> {status}: {e}")

if __name__ == "__main__":
    load_dotenv()
    id = "8f7a302f-ec99-457b-8fe2-20476f50fe4b"
    _db_update_video_status(id, "failed")

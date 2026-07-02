// Calls go-video's HTTP API directly (see ../../go-video/internal/httpapi/server.go),
// the same way the Next.js app does in app/src/app/api/video/generation/*/route.ts —
// but skipping that route's session auth, credit balance check, and its own DB writes.

export type NarrationPayload = {
  id: string;
  script: string;
  title?: string;
  bg_video: string;
  voice: string;
  music?: string;
  free_trial?: boolean;
};

export type AskRedditPayload = {
  id: string;
  title: string;
  comments: string[];
  bg_video: string;
  voice: string;
  music?: string;
  free_trial?: boolean;
};

async function post(path: string, body: unknown) {
  const base = (import.meta.env.VIDEO_SERVER_URL ?? "").replace(/\/$/, "");
  const res = await fetch(`${base}${path}`, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      Authorization: `Bearer ${import.meta.env.VIDEO_SERVER_SECRET}`,
    },
    body: JSON.stringify(body),
  });
  const text = await res.text();
  let json: unknown = undefined;
  try {
    json = JSON.parse(text);
  } catch {
    // go-video always replies JSON; fall through with raw text for error reporting.
  }
  return { ok: res.ok, status: res.status, body: json ?? text };
}

export function submitNarration(payload: NarrationPayload) {
  return post("/generation/narration", payload);
}

export function submitAskReddit(payload: AskRedditPayload) {
  return post("/generation/askreddit", payload);
}

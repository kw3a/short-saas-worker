import type { APIRoute } from "astro";
import { randomUUID } from "node:crypto";
import { BG_VIDEOS, MUSICS, VOICES } from "../../lib/constants";
import { createStubVideoRow } from "../../lib/db";
import { submitAskReddit, submitNarration } from "../../lib/videoServer";

export const prerender = false;

function badRequest(message: string) {
  return new Response(JSON.stringify({ error: message }), {
    status: 400,
    headers: { "content-type": "application/json" },
  });
}

export const POST: APIRoute = async ({ request }) => {
  const body = await request.json().catch(() => null);
  if (!body || typeof body !== "object") return badRequest("invalid JSON body");

  const { type, bgVideo, voice, music, freeTrial } = body as Record<string, unknown>;
  if (type !== "narration" && type !== "askreddit") return badRequest("type must be narration or askreddit");
  if (typeof bgVideo !== "string" || !BG_VIDEOS.includes(bgVideo as any)) return badRequest("bg_video is not valid");
  if (typeof voice !== "string" || !VOICES.includes(voice as any)) return badRequest("voice is not valid");
  if (music !== undefined && music !== "" && !MUSICS.includes(music as any)) return badRequest("music is not valid");

  const id = randomUUID();
  // Inserted before calling go-video (unlike the Next.js app's route, which
  // inserts after) so status polling never races the worker's first update —
  // this is our own local dev DB, so ordering here has no user-facing cost.
  await createStubVideoRow(id, type);

  if (type === "narration") {
    const { script, title } = body as Record<string, unknown>;
    if (typeof script !== "string" || script.trim().length < 1 || script.trim().length > 2000) {
      return badRequest("script must be between 1 and 2000 characters");
    }
    const result = await submitNarration({
      id,
      script,
      title: typeof title === "string" && title.trim() ? title : undefined,
      bg_video: bgVideo,
      voice,
      music: typeof music === "string" && music ? music : undefined,
      free_trial: Boolean(freeTrial),
    });
    if (!result.ok) return badRequest(`go-video rejected the job: ${JSON.stringify(result.body)}`);
  } else {
    const { title, comments } = body as Record<string, unknown>;
    if (typeof title !== "string" || title.trim().length < 1 || title.trim().length > 100) {
      return badRequest("title must be between 1 and 100 characters");
    }
    if (!Array.isArray(comments) || comments.length < 1 || comments.length > 20 || comments.some((c) => typeof c !== "string")) {
      return badRequest("comments must be an array of 1 to 20 strings");
    }
    const result = await submitAskReddit({
      id,
      title,
      comments: comments as string[],
      bg_video: bgVideo,
      voice,
      music: typeof music === "string" && music ? music : undefined,
      free_trial: Boolean(freeTrial),
    });
    if (!result.ok) return badRequest(`go-video rejected the job: ${JSON.stringify(result.body)}`);
  }

  return new Response(JSON.stringify({ jobId: id }), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
};

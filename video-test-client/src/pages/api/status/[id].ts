import type { APIRoute } from "astro";
import { getVideoState } from "../../../lib/db";
import { getThumbUrlIfReady, getVideoUrlIfReady } from "../../../lib/r2";

export const prerender = false;

export const GET: APIRoute = async ({ params }) => {
  const id = params.id;
  if (!id) return new Response(JSON.stringify({ error: "missing id" }), { status: 400 });

  const state = await getVideoState(id);
  if (!state) return new Response(JSON.stringify({ error: "unknown job" }), { status: 404 });

  let videoUrl: string | null = null;
  let thumbUrl: string | null = null;
  if (state.status === "completed") {
    [videoUrl, thumbUrl] = await Promise.all([getVideoUrlIfReady(id), getThumbUrlIfReady(id)]);
  }

  return new Response(JSON.stringify({ ...state, videoUrl, thumbUrl }), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
};

type StatusResponse = { status: string; progress: number; videoUrl: string | null; thumbUrl: string | null };

const POLL_INTERVAL_MS = 1000;
const MAX_POLLS = 600; // ~10 minutes

export function renderStatus(resultEl: HTMLElement, jobId: string, s: StatusResponse | { error: string }) {
  if ("error" in s) {
    resultEl.innerHTML = `<p class="error">Job ${jobId}: ${s.error}</p>`;
    return;
  }
  const dotClass = s.status === "completed" ? "completed" : s.status === "failed" ? "failed" : "rendering";
  const pct = s.status === "completed" ? 100 : Math.max(0, Math.min(100, s.progress));

  let html = `<div class="status-line"><span class="dot ${dotClass}"></span><span>Job <code>${jobId}</code> — ${s.status}</span></div>`;
  html += `
    <div class="progress-row">
      <progress max="100" value="${pct}"></progress>
      <span class="progress-pct">${pct}%</span>
    </div>`;
  if (s.status === "completed" && s.videoUrl) {
    html += `<video controls src="${s.videoUrl}"></video>`;
    if (s.thumbUrl) html += `<img alt="thumbnail" src="${s.thumbUrl}" />`;
  }
  if (s.status === "failed") {
    html += `<p class="error">Render failed — check the go-video server logs for details.</p>`;
  }
  resultEl.innerHTML = html;
}

export function startPolling(jobId: string, resultEl: HTMLElement) {
  let polls = 0;
  const tick = async () => {
    polls += 1;
    try {
      const res = await fetch(`/api/status/${jobId}`);
      const body = (await res.json()) as StatusResponse | { error: string };
      renderStatus(resultEl, jobId, body);
      if (!res.ok || (!("error" in body) && (body.status === "completed" || body.status === "failed"))) return;
    } catch (err) {
      renderStatus(resultEl, jobId, { error: String(err) });
      return;
    }
    if (polls < MAX_POLLS) setTimeout(tick, POLL_INTERVAL_MS);
  };
  void tick();
}

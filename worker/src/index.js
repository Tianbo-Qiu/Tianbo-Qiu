export default {
  async fetch(req, env) {
    if (new URL(req.url).pathname !== "/views.svg") {
      return new Response("not found", { status: 404 });
    }
    const { n } = await env.DB.prepare(
      `INSERT INTO counter (id, n) VALUES ('profile', 1)
       ON CONFLICT(id) DO UPDATE SET n = n + 1
       RETURNING n`
    ).first();
    return new Response(svg(n), {
      headers: {
        "content-type": "image/svg+xml; charset=utf-8",
        "cache-control": "no-cache, no-store, must-revalidate, max-age=0",
      },
    });
  },
};

function svg(n) {
  const label = `${n.toLocaleString("en-US")} ${n === 1 ? "visitor" : "visitors"}`;
  return `<svg xmlns="http://www.w3.org/2000/svg" width="840" height="20" role="img" aria-label="${label}">
  <style>
    text { font: 12px ui-monospace, SFMono-Regular, Menlo, monospace; fill: #59636e; }
    @media (prefers-color-scheme: dark) { text { fill: #6e7681; } }
  </style>
  <text x="800" y="14" text-anchor="end">${label}</text>
</svg>`;
}

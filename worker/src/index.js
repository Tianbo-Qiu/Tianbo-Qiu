// Text color per README theme; matches the widget's "updated" footer.
const FILL = { light: "#59636e", dark: "#6e7681" };

export default {
  async fetch(req, env) {
    const url = new URL(req.url);
    if (url.pathname !== "/views.svg") {
      return new Response("not found", { status: 404 });
    }
    const { n } = await env.DB.prepare(
      `INSERT INTO counter (id, n) VALUES ('profile', 1)
       ON CONFLICT(id) DO UPDATE SET n = n + 1
       RETURNING n`
    ).first();
    return new Response(svg(n, url.searchParams.get("theme")), {
      headers: {
        "content-type": "image/svg+xml; charset=utf-8",
        "cache-control": "no-cache, no-store, must-revalidate, max-age=0",
      },
    });
  },
};

// With ?theme=dark|light the color is fixed, so the README's <picture> decides
// (it follows the GitHub theme). Without it, fall back to the OS setting.
function svg(n, theme) {
  const label = `${n.toLocaleString("en-US")} ${n === 1 ? "visitor" : "visitors"}`;
  const style = FILL[theme]
    ? `text { font: 12px ui-monospace, SFMono-Regular, Menlo, monospace; fill: ${FILL[theme]}; }`
    : `text { font: 12px ui-monospace, SFMono-Regular, Menlo, monospace; fill: ${FILL.light}; }
    @media (prefers-color-scheme: dark) { text { fill: ${FILL.dark}; } }`;
  return `<svg xmlns="http://www.w3.org/2000/svg" width="840" height="20" role="img" aria-label="${label}">
  <style>
    ${style}
  </style>
  <text x="800" y="14" text-anchor="end">${label}</text>
</svg>`;
}

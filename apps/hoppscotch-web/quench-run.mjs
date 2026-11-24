// Starts a Hoppscotch frontend on a read-only rootfs. /site links to /tmp/hoppscotch-site, so the built site
// (/opt/hoppscotch/<name>) is copied there and the VITE_* environment is written into the copy, where upstream's
// prod_run.mjs runs @import-meta-env/cli in place. Then it serves the copy the way upstream's Caddyfile does
// (try_files {path} /: a file, else index.html), web on 3000 and admin on 3100, plus webapp-server (the desktop-app
// bundle on 3200) when the image has one. Wolfi's public caddy lags its Go fixes, and Node is here anyway.
import { spawn } from "node:child_process"
import fs from "node:fs"
import http from "node:http"
import path from "node:path"

const SRC = "/opt/hoppscotch", SITE = "/tmp/hoppscotch-site"
const PORTS = { "selfhost-web": 3000, "sh-admin": 3100 }
const TYPES = {
  ".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".mjs": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8", ".json": "application/json", ".map": "application/json",
  ".webmanifest": "application/manifest+json", ".svg": "image/svg+xml", ".png": "image/png", ".jpg": "image/jpeg",
  ".ico": "image/x-icon", ".webp": "image/webp", ".woff": "font/woff", ".woff2": "font/woff2", ".ttf": "font/ttf",
  ".wasm": "application/wasm", ".txt": "text/plain; charset=utf-8", ".xml": "application/xml",
}

// ponytail: no ETag or compression (Caddy's file_server without encode sends no compression either); add them if
// load times matter behind an ingress that does not compress.
export function resolve(root, url) {
  let p
  try { p = decodeURIComponent(new URL(url, "http://x").pathname) } catch { return null }
  const f = path.join(root, p)
  if (f !== root && !f.startsWith(root + path.sep)) return null
  return fs.statSync(f, { throwIfNoEntry: false })?.isFile() ? f : path.join(root, "index.html")
}
function serve(root, port) {
  return http.createServer((req, res) => {
    if (req.method !== "GET" && req.method !== "HEAD") return res.writeHead(405).end()
    const f = resolve(root, req.url)
    if (!f) return res.writeHead(400).end()
    res.writeHead(200, { "Content-Type": TYPES[path.extname(f)] ?? "application/octet-stream",
      "Content-Length": fs.statSync(f).size, "X-Content-Type-Options": "nosniff" })
    if (req.method === "HEAD") return res.end()
    fs.createReadStream(f).pipe(res)
  }).listen(port, () => console.log(`serving ${root} on :${port}`))
}

// The placeholder @import-meta-env/unplugin leaves in the build and the CLI's replacement, ported from
// @import-meta-env/cli 0.7.4 (whose bundled serialize-javascript 6.0.2 carries three advisories). For an object of
// strings serialize-javascript is JSON.stringify with these five characters escaped.
const PH = `JSON.parse('"import_meta_env_placeholder"')`
const ESC = { "<": "\\u003C", ">": "\\u003E", "/": "\\u002F", "\u2028": "\\u2028", "\u2029": "\\u2029" }
const serialize = (o) => JSON.stringify(o).replace(/[<>/\u2028\u2029]/g, (c) => ESC[c])
const slash = (c, n) => "\\".repeat(n * 2) + c
const placeholder = (dq, sq) =>
  new RegExp(PH.replace(/\\/g, "\\\\").replace(/([()])/g, "\\$1").replace(/"/g, slash('"', dq)).replace(/'/g, slash("'", sq)), "g")
export function inject(code, env) {
  const escaped = Object.fromEntries(Object.entries(env).map(([k, v]) => [k, v.replace(/\\/g, "\\\\").replace(/"/g, '\\"')]))
  const s = serialize(escaped)
  return code
    .replace(placeholder(2, 1), () => `JSON.parse(\\'${s.replace(/\\/g, "\\\\").replace(/"/g, '\\\\"')}\\')`)
    .replace(placeholder(1, 0), () => `JSON.parse('${s.replace(/\\/g, "\\\\").replace(/"/g, '\\"')}')`)
    .replace(placeholder(0, 0), () => `JSON.parse('${s}')`)
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const env = Object.fromEntries(Object.entries(process.env).filter(([k]) => k.startsWith("VITE_")).sort(([a], [b]) => a.localeCompare(b)))
  fs.rmSync(SITE, { recursive: true, force: true })
  for (const name of fs.readdirSync(SRC).filter((n) => fs.statSync(`${SRC}/${n}`).isDirectory())) {
    fs.cpSync(`${SRC}/${name}`, `${SITE}/${name}`, { recursive: true })
  }
  for (const f of fs.readdirSync(SITE, { recursive: true }).map((f) => path.join(SITE, f))) {
    if (f.endsWith(".map") || !fs.statSync(f).isFile()) continue
    const code = fs.readFileSync(f, "utf8")
    if (code.includes("import_meta_env_placeholder")) fs.writeFileSync(f, inject(code, env))
  }

  for (const [name, port] of Object.entries(PORTS)) if (fs.existsSync(`${SITE}/${name}`)) serve(`${SITE}/${name}`, port)
  const kids = fs.existsSync("/usr/bin/webapp-server") ? [spawn("/usr/bin/webapp-server", [], { stdio: "inherit" })] : []
  for (const k of kids) k.on("exit", (code, sig) => process.exit(code ?? (sig ? 1 : 0)))
  for (const sig of ["SIGTERM", "SIGINT"]) process.on(sig, () => { kids.forEach((k) => k.kill(sig)); process.exit(0) })
}

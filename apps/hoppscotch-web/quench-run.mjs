// Starts a Hoppscotch frontend on a read-only rootfs. /site links to /tmp/hoppscotch-site, so the built site
// (/opt/hoppscotch/<name>) is copied there and the VITE_* environment is written into the copy, where upstream's
// prod_run.mjs runs @import-meta-env/cli in place. Then Caddy serves it, with webapp-server (the desktop-app bundle
// on 3200) when the image has one; the container exits when either child does.
import { spawn } from "node:child_process"
import fs from "node:fs"
import path from "node:path"

const SRC = "/opt/hoppscotch", SITE = "/tmp/hoppscotch-site"

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

  const kids = [spawn("/usr/bin/caddy", ["run", "--config", `${SRC}/Caddyfile`, "--adapter", "caddyfile"], { stdio: "inherit" })]
  if (fs.existsSync("/usr/bin/webapp-server")) kids.push(spawn("/usr/bin/webapp-server", [], { stdio: "inherit" }))
  for (const sig of ["SIGTERM", "SIGINT"]) process.on(sig, () => kids.forEach((k) => k.kill(sig)))
  for (const k of kids) {
    k.on("exit", (code, sig) => {
      if (process.exitCode === undefined) process.exitCode = code ?? (sig ? 1 : 0)
      kids.forEach((o) => o.kill("SIGTERM"))
    })
  }
}

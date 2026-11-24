// Starts a Hoppscotch frontend on a read-only rootfs. /site links to /tmp/hoppscotch-site, so the built site
// (/opt/hoppscotch/<name>) is copied there and import-meta-env writes the VITE_* environment into the copy, the
// way upstream's prod_run.mjs does in place. Then Caddy serves it, with webapp-server (the desktop-app bundle on
// 3200) when the image has one; the container exits when either child does.
import { execFileSync, spawn } from "node:child_process"
import fs from "node:fs"

const SRC = "/opt/hoppscotch", SITE = "/tmp/hoppscotch-site"
fs.rmSync(SITE, { recursive: true, force: true })
for (const name of fs.readdirSync(SRC).filter((n) => fs.statSync(`${SRC}/${n}`).isDirectory() && n !== "cli")) {
  fs.cpSync(`${SRC}/${name}`, `${SITE}/${name}`, { recursive: true })
}

const env = Object.entries(process.env)
  .filter(([k]) => k.startsWith("VITE_"))
  .sort(([a], [b]) => a.localeCompare(b))
  .map(([k, v]) => `${k}=${v.startsWith('"') && v.endsWith('"') ? v : `"${v}"`}`)
  .join("\n")
const envFile = `${fs.mkdtempSync("/tmp/hopp-env-")}/build.env`
fs.writeFileSync(envFile, env)
try {
  execFileSync("/usr/bin/node", [`${SRC}/cli/node_modules/@import-meta-env/cli/bin/import-meta-env.js`,
    "-x", envFile, "-e", envFile, "-p", `${SITE}/**/*`], { stdio: "inherit" })
} finally {
  fs.rmSync(envFile, { force: true })
}

const kids = [spawn("/usr/bin/caddy", ["run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"], { stdio: "inherit" })]
if (fs.existsSync("/usr/bin/webapp-server")) kids.push(spawn("/usr/bin/webapp-server", [], { stdio: "inherit" }))
for (const sig of ["SIGTERM", "SIGINT"]) process.on(sig, () => kids.forEach((k) => k.kill(sig)))
for (const k of kids) {
  k.on("exit", (code, sig) => {
    if (process.exitCode === undefined) process.exitCode = code ?? (sig ? 1 : 0)
    kids.forEach((o) => o.kill("SIGTERM"))
  })
}

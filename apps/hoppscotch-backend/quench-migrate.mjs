// Applies prisma/migrations the way `prisma migrate deploy` does, without Prisma's Rust schema engine (images#165):
// Prisma's advisory lock, its _prisma_migrations table, one row per migration (checksum = sha256 of migration.sql)
// inserted before the SQL runs and finished after, so a later Prisma Migrate sees the same history. A failed
// migration stays recorded unfinished and stops later runs, as Prisma does. Usage: node quench-migrate.mjs (DATABASE_URL).
import { createHash, randomUUID } from 'node:crypto';
import { readdirSync, readFileSync, existsSync } from 'node:fs';
import { createRequire } from 'node:module';
import { join } from 'node:path';

const require = createRequire(import.meta.url);
let pg;
try { pg = require('pg'); } catch { pg = createRequire(require.resolve('@prisma/adapter-pg'))('pg'); }

const dir = join(import.meta.dirname, 'prisma', 'migrations');
const client = new pg.Client({ connectionString: process.env.DATABASE_URL });
await client.connect();
try {
  await client.query('SELECT pg_advisory_lock(72707369)'); // Prisma Migrate's lock key
  await client.query(`CREATE TABLE IF NOT EXISTS "_prisma_migrations" (
    "id" VARCHAR(36) PRIMARY KEY NOT NULL, "checksum" VARCHAR(64) NOT NULL, "finished_at" TIMESTAMPTZ,
    "migration_name" VARCHAR(255) NOT NULL, "logs" TEXT, "rolled_back_at" TIMESTAMPTZ,
    "started_at" TIMESTAMPTZ NOT NULL DEFAULT now(), "applied_steps_count" INTEGER NOT NULL DEFAULT 0)`);
  const { rows } = await client.query('SELECT migration_name, finished_at, rolled_back_at FROM "_prisma_migrations"');
  const failed = rows.filter((r) => !r.finished_at && !r.rolled_back_at).map((r) => r.migration_name);
  if (failed.length) throw new Error(`failed migrations in the database, resolve them first: ${failed.join(', ')}`);
  const done = new Set(rows.filter((r) => r.finished_at).map((r) => r.migration_name));
  const names = readdirSync(dir).filter((n) => existsSync(join(dir, n, 'migration.sql'))).sort();
  let applied = 0;
  for (const name of names) {
    if (done.has(name)) continue;
    const sql = readFileSync(join(dir, name, 'migration.sql'), 'utf8');
    const id = randomUUID();
    const checksum = createHash('sha256').update(sql).digest('hex');
    await client.query('INSERT INTO "_prisma_migrations" (id, checksum, migration_name) VALUES ($1, $2, $3)', [id, checksum, name]);
    try {
      await client.query(sql); // one multi-statement simple query: PostgreSQL runs it as one implicit transaction
    } catch (e) {
      await client.query('UPDATE "_prisma_migrations" SET logs = $2 WHERE id = $1', [id, String(e.stack || e)]);
      throw new Error(`migration ${name} failed: ${e.message}`);
    }
    await client.query('UPDATE "_prisma_migrations" SET finished_at = now(), applied_steps_count = 1 WHERE id = $1', [id]);
    console.log(`applied ${name}`);
    applied++;
  }
  console.log(applied ? `${applied} migration(s) applied` : 'No pending migrations to apply.');
} finally {
  await client.query('SELECT pg_advisory_unlock(72707369)').catch(() => {});
  await client.end();
}

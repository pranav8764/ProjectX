/**
 * Seed BetterAuth credential accounts for the demo persona users so they can log in
 * with email + password. The persona rows already exist in identity.users (with their
 * org membership + role) from infra/db/seeds/demo_seed.sql, but have no password/account.
 *
 * This creates an identity.accounts row (provider_id='credential') per persona, with the
 * password hashed exactly the way BetterAuth's credential provider verifies it.
 *
 * Usage (from repo root, with .env present):
 *   node apps/web/scripts/seed-demo-auth.mjs
 *   DEMO_PASSWORD='Custom#Pass1' node apps/web/scripts/seed-demo-auth.mjs
 *
 * Idempotent: skips a persona that already has a credential account.
 */
import fs from 'fs';
import { betterAuth } from 'better-auth';
import pg from 'pg';

// Load repo-root .env (this file lives at apps/web/scripts/)
const envPath = new URL('../../../.env', import.meta.url);
if (fs.existsSync(envPath)) {
  for (const line of fs.readFileSync(envPath, 'utf8').split('\n')) {
    const t = line.trim();
    if (!t || t.startsWith('#') || !t.includes('=')) continue;
    const i = t.indexOf('=');
    const k = t.slice(0, i);
    const v = t.slice(i + 1);
    if (!(k in process.env)) process.env[k] = v;
  }
}

if (!process.env.DATABASE_URL) {
  console.error('DATABASE_URL is not set (copy .env.example to .env).');
  process.exit(1);
}

const DEMO_PASSWORD = process.env.DEMO_PASSWORD || 'PlantBrain#2026';
const PERSONAS = [
  'arjun@plantbrain.corp',
  'ravi@plantbrain.corp',
  'meera@plantbrain.corp',
  'suresh@plantbrain.corp',
];

const pool = new pg.Pool({
  connectionString: process.env.DATABASE_URL,
  options: '-c search_path=identity,public',
  max: 5,
});

// Minimal auth instance — only needed for its password hasher, which matches what
// the credential provider uses to verify at sign-in.
const auth = betterAuth({
  database: pool,
  secret: process.env.BETTER_AUTH_SECRET || 'seed-only-not-used-for-hashing',
  emailAndPassword: { enabled: true },
});

async function main() {
  const ctx = await auth.$context;
  const hash = await ctx.password.hash(DEMO_PASSWORD);

  const client = await pool.connect();
  let created = 0;
  let skipped = 0;
  try {
    for (const email of PERSONAS) {
      const userRow = await client.query('SELECT id FROM identity.users WHERE email = $1 LIMIT 1', [email]);
      const userId = userRow.rows[0]?.id;
      if (!userId) {
        console.log(`  - ${email}: no user row (run demo seed first) — skipped`);
        skipped++;
        continue;
      }
      const existing = await client.query(
        "SELECT 1 FROM identity.accounts WHERE user_id = $1 AND provider_id = 'credential' LIMIT 1",
        [userId],
      );
      if (existing.rowCount > 0) {
        console.log(`  - ${email}: credential account already exists — skipped`);
        skipped++;
        continue;
      }
      await client.query(
        `INSERT INTO identity.accounts (id, account_id, provider_id, user_id, password, created_at, updated_at)
         VALUES ($1, $2, 'credential', $3, $4, NOW(), NOW())`,
        [crypto.randomUUID(), userId, userId, hash],
      );
      console.log(`  - ${email}: credential account created`);
      created++;
    }
  } finally {
    client.release();
    await pool.end();
  }

  console.log(`\nDone. ${created} created, ${skipped} skipped.`);
  console.log(`Demo login password: ${DEMO_PASSWORD}`);
  console.log('Sign in at /login with any persona email above.');
}

main().catch((e) => {
  console.error('seed-demo-auth failed:', e?.message || e);
  process.exit(1);
});

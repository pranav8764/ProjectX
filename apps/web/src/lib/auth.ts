import { betterAuth } from 'better-auth';
import { Pool } from 'pg';

// Single pool for the auth adapter. `search_path=identity` resolves BetterAuth's
// unqualified table names to our identity.* tables (mapped to plural names below).
const pool = new Pool({
  connectionString: process.env.DATABASE_URL,
  options: '-c search_path=identity,public',
  max: 5,
});

// Default org + role that a brand-new email/password signup joins. The Go gateway
// resolves org/role from users.organization_id + memberships, so a user with neither
// cannot use the app — the after-signup hook backfills both.
const DEFAULT_ORG_NAME = 'PlantBrain Corp';
const DEFAULT_ROLE_NAME = 'Engineer';

export const auth = betterAuth({
  database: pool,
  secret: process.env.BETTER_AUTH_SECRET,
  baseURL: process.env.BETTER_AUTH_URL || 'http://localhost:3000',
  // identity.users.id is a uuid column; BetterAuth's default base32 ids fail to insert.
  advanced: {
    database: {
      generateId: () => globalThis.crypto.randomUUID(),
    },
  },
  emailAndPassword: {
    enabled: true,
    requireEmailVerification: false,
    autoSignIn: true,
  },
  user: {
    modelName: 'users',
    fields: {
      emailVerified: 'email_verified',
      createdAt: 'created_at',
      updatedAt: 'updated_at',
    },
    additionalFields: {
      organization_id: { type: 'string', required: false, input: false },
      mobile_no: { type: 'string', required: false, input: false },
    },
  },
  session: {
    modelName: 'sessions',
    fields: {
      userId: 'user_id',
      expiresAt: 'expires_at',
      createdAt: 'created_at',
      updatedAt: 'updated_at',
      ipAddress: 'ip_address',
      userAgent: 'user_agent',
    },
  },
  account: {
    modelName: 'accounts',
    fields: {
      userId: 'user_id',
      accountId: 'account_id',
      providerId: 'provider_id',
      accessToken: 'access_token',
      refreshToken: 'refresh_token',
      idToken: 'id_token',
      accessTokenExpiresAt: 'access_token_expires_at',
      refreshTokenExpiresAt: 'refresh_token_expires_at',
      createdAt: 'created_at',
      updatedAt: 'updated_at',
    },
  },
  verification: {
    modelName: 'verifications',
    fields: {
      expiresAt: 'expires_at',
      createdAt: 'created_at',
      updatedAt: 'updated_at',
    },
  },
  databaseHooks: {
    user: {
      create: {
        after: async (user) => {
          // Backfill organization + membership + role for a fresh signup so the
          // gateway can resolve a workspace for the new user.
          const client = await pool.connect();
          try {
            const orgRes = await client.query(
              `INSERT INTO identity.organizations (name, industry)
               SELECT $1, 'Manufacturing & Process'
               WHERE NOT EXISTS (SELECT 1 FROM identity.organizations WHERE name = $1)
               RETURNING id`,
              [DEFAULT_ORG_NAME],
            );
            const orgId =
              orgRes.rows[0]?.id ??
              (await client.query('SELECT id FROM identity.organizations WHERE name = $1 LIMIT 1', [DEFAULT_ORG_NAME]))
                .rows[0]?.id;

            const roleRes = await client.query(
              `INSERT INTO identity.roles (name, description)
               SELECT $1, 'Upload documents, run queries, and view asset intelligence/RCA.'
               WHERE NOT EXISTS (SELECT 1 FROM identity.roles WHERE name = $1)
               RETURNING id`,
              [DEFAULT_ROLE_NAME],
            );
            const roleId =
              roleRes.rows[0]?.id ??
              (await client.query('SELECT id FROM identity.roles WHERE name = $1 LIMIT 1', [DEFAULT_ROLE_NAME]))
                .rows[0]?.id;

            // Give the membership a concrete plant (the org's first) so the gateway
            // can resolve a default plantId for the user; NULL would leave plant-scoped
            // pages without a workspace to query.
            const plantRes = await client.query(
              'SELECT id FROM identity.plants WHERE organization_id = $1 ORDER BY created_at ASC LIMIT 1',
              [orgId],
            );
            const plantId = plantRes.rows[0]?.id ?? null;

            await client.query('UPDATE identity.users SET organization_id = $1 WHERE id = $2', [orgId, user.id]);
            await client.query(
              `INSERT INTO identity.memberships (organization_id, plant_id, user_id, role_id)
               SELECT $1, $2, $3, $4
               WHERE NOT EXISTS (
                 SELECT 1 FROM identity.memberships WHERE user_id = $3 AND organization_id = $1
               )`,
              [orgId, plantId, user.id, roleId],
            );
          } finally {
            client.release();
          }
        },
      },
    },
  },
});

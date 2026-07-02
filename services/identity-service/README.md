# Identity Service

Owns organizations, users, roles, memberships, and auth provider mappings.

## Responsibilities

- Organization and plant membership lookup
- User profile mapping from Clerk/BetterAuth/demo auth
- Role-based permission checks
- Internal permission decision APIs

## Owns Schema

`identity`

## Key APIs

- `GET /internal/users/{userId}`
- `GET /internal/organizations/{organizationId}/memberships`
- `POST /internal/permissions/check`


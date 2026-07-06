package main

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

// seedDevelopmentData populates base tables with development defaults to allow out-of-the-box local testing.
func seedDevelopmentData(dbPool *pgxpool.Pool) {
	ctx := context.Background()

	var count int
	err := dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM identity.organizations").Scan(&count)
	if err != nil {
		log.Printf("Warning: Failed to check identity.organizations size: %v", err)
		return
	}

	if count > 0 {
		log.Println("Database already contains data, skipping auto-seeding")
		return
	}

	log.Println("Database is empty. Seeding development defaults...")

	// 1. Organization
	var orgID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO identity.organizations (name, industry)
		VALUES ('Acme Industrial', 'Manufacturing & Refining')
		RETURNING id::text
	`).Scan(&orgID)
	if err != nil {
		log.Printf("Warning: Seeding organization failed: %v", err)
		return
	}

	// 2. Plant
	var plantID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO identity.plants (organization_id, name, location)
		VALUES ($1, 'Acme Refining Unit-1', 'Houston, TX')
		RETURNING id::text
	`, orgID).Scan(&plantID)
	if err != nil {
		log.Printf("Warning: Seeding plant failed: %v", err)
		return
	}

	// 3. User
	var userID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO identity.users (organization_id, name, email, email_verified)
		VALUES ($1, 'Primary Operator', 'operator@acme.com', true)
		RETURNING id::text
	`, orgID).Scan(&userID)
	if err != nil {
		log.Printf("Warning: Seeding user failed: %v", err)
		return
	}

	// 4. Role
	var roleID string
	err = dbPool.QueryRow(ctx, `
		INSERT INTO identity.roles (name, description)
		VALUES ('engineer', 'Plant engineer with search and RCA access')
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id::text
	`).Scan(&roleID)
	if err != nil {
		// Attempt selecting if update did not trigger RETURNING properly on conflict
		err = dbPool.QueryRow(ctx, "SELECT id::text FROM identity.roles WHERE name = 'engineer'").Scan(&roleID)
		if err != nil {
			log.Printf("Warning: Seeding/fetching role failed: %v", err)
			return
		}
	}

	// 5. Membership
	_, err = dbPool.Exec(ctx, `
		INSERT INTO identity.memberships (organization_id, plant_id, user_id, role_id)
		VALUES ($1, $2, $3, $4)
	`, orgID, plantID, userID, roleID)
	if err != nil {
		log.Printf("Warning: Seeding membership failed: %v", err)
		return
	}

	// 6. Session (with dev-token)
	_, err = dbPool.Exec(ctx, `
		INSERT INTO identity.sessions (id, token, user_id, expires_at)
		VALUES ('dev-session-id', 'dev-token', $1, NOW() + INTERVAL '30 days')
		ON CONFLICT (id) DO NOTHING
	`, userID)
	if err != nil {
		log.Printf("Warning: Seeding session failed: %v", err)
		return
	}

	// 7. Seed sample Asset (Pump P-101)
	_, err = dbPool.Exec(ctx, `
		INSERT INTO asset.assets (organization_id, plant_id, asset_tag, asset_name, asset_type, location, criticality, risk_score)
		VALUES ($1, $2, 'P-101', 'Cooling Water Pump P-101', 'Pump', 'Unit-2', 'HIGH', 78)
		ON CONFLICT (plant_id, asset_tag) DO NOTHING
	`, orgID, plantID)
	if err != nil {
		log.Printf("Warning: Seeding sample asset P-101 failed: %v", err)
		return
	}

	log.Println("Seeding completed successfully! Dev token is 'dev-token', Plant ID is:", plantID)
}

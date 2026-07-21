package handlers

import (
	"fmt"
	"log"
	"net/http"

	"plantbrain-api/internal/auth"
	"plantbrain-api/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HandleGetAssets retrieves asset profiles, optionally filtered by plant.
func HandleGetAssets(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("orgID")
		plantID := c.Query("plantId")
		if plantID == "" {
			plantID = c.GetString("plantID")
		}
		if plantID != "" {
			if _, err := uuid.Parse(plantID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plantId format"})
				return
			}
			if !auth.RequirePlantAccess(c, dbPool, plantID) {
				return
			}
		}

		var rows pgx.Rows
		var err error
		ctx := c.Request.Context()

		if plantID != "" {
			rows, err = dbPool.Query(ctx, `
				SELECT id, organization_id, plant_id, asset_tag, asset_name, asset_type, location, criticality, risk_score, created_at, updated_at
				FROM asset.assets
				WHERE organization_id = $1 AND plant_id = $2
				ORDER BY asset_tag ASC
			`, orgID, plantID)
		} else {
			rows, err = dbPool.Query(ctx, `
				SELECT id, organization_id, plant_id, asset_tag, asset_name, asset_type, location, criticality, risk_score, created_at, updated_at
				FROM asset.assets
				WHERE organization_id = $1
				ORDER BY asset_tag ASC
			`, orgID)
		}

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to retrieve assets: %v", err)})
			return
		}
		defer rows.Close()

		assets := []models.Asset{}
		for rows.Next() {
			var a models.Asset
			var name, aType, loc, crit *string
			var score *float64
			err := rows.Scan(
				&a.ID, &a.OrganizationID, &a.PlantID, &a.AssetTag,
				&name, &aType, &loc, &crit, &score,
				&a.CreatedAt, &a.UpdatedAt,
			)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to scan asset: %v", err)})
				return
			}
			if name != nil {
				a.AssetName = *name
			}
			if aType != nil {
				a.AssetType = *aType
			}
			if loc != nil {
				a.Location = *loc
			}
			if crit != nil {
				a.Criticality = *crit
			}
			if score != nil {
				a.RiskScore = *score
			}
			assets = append(assets, a)
		}
		if err := rows.Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "error iterating assets"})
			return
		}

		c.JSON(http.StatusOK, assets)
	}
}

// HandleGetAssetByID retrieves a detailed asset profile by ID or Tag.
func HandleGetAssetByID(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		assetLookup := c.Param("id")
		orgID := c.GetString("orgID")
		plantID := c.Query("plantId")
		if plantID == "" {
			plantID = c.GetString("plantID")
		}
		if plantID != "" {
			if _, err := uuid.Parse(plantID); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plantId format"})
				return
			}
		}
		ctx := c.Request.Context()

		var a models.Asset
		var name, aType, loc, crit *string
		var score *float64

		var err error
		if _, parseErr := uuid.Parse(assetLookup); parseErr == nil {
			err = dbPool.QueryRow(ctx, `
				SELECT id, organization_id, plant_id, asset_tag, asset_name, asset_type, location, criticality, risk_score, created_at, updated_at
				FROM asset.assets
				WHERE id = $1 AND organization_id = $2 AND ($3::uuid IS NULL OR plant_id = $3::uuid)
			`, assetLookup, orgID, nullableUUID(plantID)).Scan(
				&a.ID, &a.OrganizationID, &a.PlantID, &a.AssetTag,
				&name, &aType, &loc, &crit, &score,
				&a.CreatedAt, &a.UpdatedAt,
			)
		} else {
			err = dbPool.QueryRow(ctx, `
				SELECT id, organization_id, plant_id, asset_tag, asset_name, asset_type, location, criticality, risk_score, created_at, updated_at
				FROM asset.assets
				WHERE UPPER(asset_tag) = UPPER($1) AND organization_id = $2 AND ($3::uuid IS NULL OR plant_id = $3::uuid)
				ORDER BY updated_at DESC
				LIMIT 1
			`, assetLookup, orgID, nullableUUID(plantID)).Scan(
				&a.ID, &a.OrganizationID, &a.PlantID, &a.AssetTag,
				&name, &aType, &loc, &crit, &score,
				&a.CreatedAt, &a.UpdatedAt,
			)
		}

		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Asset not found"})
			return
		}
		if !auth.RequirePlantAccess(c, dbPool, a.PlantID) {
			return
		}

		if name != nil {
			a.AssetName = *name
		}
		if aType != nil {
			a.AssetType = *aType
		}
		if loc != nil {
			a.Location = *loc
		}
		if crit != nil {
			a.Criticality = *crit
		}
		if score != nil {
			a.RiskScore = *score
		}

		// Fetch failures (RCA reports)
		failures := []models.Failure{}
		fRows, err := dbPool.Query(ctx, `
			SELECT id::text, failure_summary, timeline, probable_causes, recommendations, confidence, created_by::text, created_at
			FROM rca.reports
			WHERE asset_id = $1 AND organization_id = $2
			ORDER BY created_at DESC
		`, a.ID, orgID)
		if err == nil {
			defer fRows.Close()
			for fRows.Next() {
				var f models.Failure
				var timelineJSON, causesJSON, recsJSON []byte
				var createdBy *string
				var confidence *float64
				err := fRows.Scan(&f.ID, &f.FailureSummary, &timelineJSON, &causesJSON, &recsJSON, &confidence, &createdBy, &f.CreatedAt)
				if err == nil {
					if confidence != nil {
						f.Confidence = *confidence
					}
					if createdBy != nil {
						f.CreatedBy = createdBy
					}
					f.Timeline = jsonTextList(timelineJSON)
					f.ProbableCauses = jsonTextList(causesJSON)
					f.Recommendations = jsonTextList(recsJSON)
					f.Source = "RCA"
					failures = append(failures, f)
				} else {
					log.Printf("Failed to scan RCA report: %v", err)
				}
			}
			if err := fRows.Err(); err != nil {
				log.Printf("Error iterating RCA reports: %v", err)
			}
		} else {
			log.Printf("Failed to query RCA reports: %v", err)
		}

		// Fetch compliance gaps
		gaps := []models.Gap{}
		gRows, err := dbPool.Query(ctx, `
			SELECT id, gap_type, description, severity, status, created_at, updated_at
			FROM compliance.gaps
			WHERE asset_id = $1 AND organization_id = $2
			ORDER BY created_at DESC
		`, a.ID, orgID)
		if err == nil {
			defer gRows.Close()
			for gRows.Next() {
				var g models.Gap
				var severity, status *string
				err := gRows.Scan(&g.ID, &g.GapType, &g.Description, &severity, &status, &g.CreatedAt, &g.UpdatedAt)
				if err == nil {
					if severity != nil {
						g.Severity = *severity
					}
					if status != nil {
						g.Status = *status
					}
					gaps = append(gaps, g)
				} else {
					log.Printf("Failed to scan compliance gap: %v", err)
				}
			}
			if err := gRows.Err(); err != nil {
				log.Printf("Error iterating compliance gaps: %v", err)
			}
		} else {
			log.Printf("Failed to query compliance gaps: %v", err)
		}

		// Fetch linked documents via Graph Entities matching this asset's tag
		documents := []models.Document{}
		dRows, err := dbPool.Query(ctx, `
			SELECT DISTINCT d.id::text, d.title, d.document_type, d.status, d.created_at
			FROM graph.entities e
			JOIN document.documents d ON e.document_id = d.id
			WHERE e.normalized_value = $1 AND e.organization_id = $2
			  AND d.plant_id = $3
			  AND d.status <> 'ARCHIVED'
			ORDER BY d.created_at DESC
		`, a.AssetTag, orgID, a.PlantID)
		if err == nil {
			defer dRows.Close()
			for dRows.Next() {
				var d models.Document
				var docType *string
				err := dRows.Scan(&d.ID, &d.Title, &docType, &d.Status, &d.CreatedAt)
				if err == nil {
					if docType != nil {
						d.DocumentType = *docType
					}
					documents = append(documents, d)
				} else {
					log.Printf("Failed to scan linked document: %v", err)
				}
			}
			if err := dRows.Err(); err != nil {
				log.Printf("Error iterating linked documents: %v", err)
			}
		} else {
			log.Printf("Failed to query linked documents: %v", err)
		}

		c.JSON(http.StatusOK, gin.H{
			"id":             a.ID,
			"organizationId": a.OrganizationID,
			"plantId":        a.PlantID,
			"assetTag":       a.AssetTag,
			"assetName":      a.AssetName,
			"assetType":      a.AssetType,
			"location":       a.Location,
			"criticality":    a.Criticality,
			"riskScore":      a.RiskScore,
			"failures":       failures,
			"documents":      documents,
			"complianceGaps": gaps,
		})
	}
}

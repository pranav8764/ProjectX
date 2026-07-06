package handlers

import (
	"fmt"
	"net/http"
	"net/url"

	"plantbrain-api/internal/auth"
	"plantbrain-api/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HandleGetComplianceGaps lists the active compliance gaps.
func HandleGetComplianceGaps(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("orgID")
		plantID := c.Query("plantId")
		if plantID == "" {
			plantID = c.GetString("plantID")
		}
		if plantID != "" && !auth.RequirePlantAccess(c, dbPool, plantID) {
			return
		}
		ctx := c.Request.Context()

		rows, err := dbPool.Query(ctx, `
			SELECT
				g.id,
				g.organization_id,
				g.plant_id,
				g.asset_id,
				a.asset_tag,
				g.requirement_id,
				g.gap_type,
				g.description,
				g.severity,
				g.evidence_document_id,
				g.status,
				g.created_at,
				g.updated_at
			FROM compliance.gaps g
			LEFT JOIN asset.assets a ON g.asset_id = a.id
			WHERE g.organization_id = $1
			  AND ($2::uuid IS NULL OR g.plant_id = $2::uuid)
			ORDER BY g.created_at DESC
		`, orgID, nullableUUID(plantID))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to retrieve compliance gaps: %v", err)})
			return
		}
		defer rows.Close()

		gaps := []models.ComplianceGap{}
		for rows.Next() {
			var g models.ComplianceGap
			err := rows.Scan(
				&g.ID, &g.OrganizationID, &g.PlantID, &g.AssetID, &g.AssetTag, &g.RequirementID,
				&g.GapType, &g.Description, &g.Severity, &g.EvidenceDocumentID, &g.Status,
				&g.CreatedAt, &g.UpdatedAt,
			)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to scan gap: %v", err)})
				return
			}
			gaps = append(gaps, g)
		}

		c.JSON(http.StatusOK, gaps)
	}
}

// HandleComplianceScan requests the AI service to run a compliance scan on the plant assets.
func HandleComplianceScan(dbPool *pgxpool.Pool, aiServiceURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		plantID := c.Query("plantId")
		if plantID == "" {
			plantID = c.GetString("plantID")
		}
		if plantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "plantId is required"})
			return
		}
		if !auth.RequirePlantAccess(c, dbPool, plantID) {
			return
		}

		recordAuditEvent(c, dbPool, plantID, "COMPLIANCE_SCAN_REQUESTED", "plant", plantID, nil)

		aiPath := fmt.Sprintf("/compliance?plantId=%s", url.QueryEscape(plantID))
		aiResp, err := callAIService(c.Request.Context(), http.MethodGet, aiServiceURL, aiPath, nil)
		if err != nil || aiResp.StatusCode < 200 || aiResp.StatusCode >= 300 {
			reason := aiFailureReason(err, aiResp)
			recordAuditEvent(c, dbPool, plantID, "COMPLIANCE_SCAN_FALLBACK", "plant", plantID, map[string]interface{}{
				"reason": reason,
			})
			c.JSON(http.StatusOK, gin.H{
				"status":      "DEGRADED",
				"fallback":    true,
				"gaps":        []gin.H{},
				"missingInfo": []string{"AI service unavailable or timed out; compliance scan did not complete", reason},
			})
			return
		}

		c.Data(aiResp.StatusCode, "application/json", aiResp.Body)
	}
}

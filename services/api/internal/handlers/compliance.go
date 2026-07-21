package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"plantbrain-api/internal/auth"
	"plantbrain-api/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

// HandleUpdateComplianceGap handles PATCH /api/compliance/gaps/:id
func HandleUpdateComplianceGap(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		gapID := c.Param("id")
		if _, err := uuid.Parse(gapID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid gap id format"})
			return
		}
		orgID := c.GetString("orgID")
		ctx := c.Request.Context()

		var body struct {
			Status string `json:"status" binding:"required"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		status := strings.ToUpper(strings.TrimSpace(body.Status))
		if status != "OPEN" && status != "CLOSED" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "status must be one of OPEN or CLOSED"})
			return
		}

		var plantID *string
		if err := dbPool.QueryRow(ctx, `
			SELECT plant_id::text
			FROM compliance.gaps
			WHERE id = $1 AND organization_id = $2
		`, gapID, orgID).Scan(&plantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "compliance gap not found"})
			return
		}
		if !auth.RequirePlantAccess(c, dbPool, stringValue(plantID)) {
			return
		}

		tag, err := dbPool.Exec(ctx, `
			UPDATE compliance.gaps
			SET status = $1, updated_at = NOW()
			WHERE id = $2 AND organization_id = $3
		`, status, gapID, orgID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update compliance gap: %v", err)})
			return
		}
		if tag.RowsAffected() == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "compliance gap not found"})
			return
		}

		recordAuditEvent(c, dbPool, stringValue(plantID), "COMPLIANCE_GAP_UPDATED", "compliance_gap", gapID, map[string]interface{}{
			"status": status,
		})

		c.JSON(http.StatusOK, gin.H{"id": gapID, "status": status})
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

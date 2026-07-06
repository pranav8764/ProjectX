package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"plantbrain-api/internal/auth"
	"plantbrain-api/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HandleRCAGenerate triggers RCA report compilation via the AI Service.
func HandleRCAGenerate(dbPool *pgxpool.Pool, aiServiceURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.RCAGenerateReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if req.PlantID == "" {
			req.PlantID = c.GetString("plantID")
		}
		req.UserID = c.GetString("userID")
		if req.PlantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "plantId is required"})
			return
		}
		if !auth.RequirePlantAccess(c, dbPool, req.PlantID) {
			return
		}
		req.OrganizationID = c.GetString("orgID")

		// Verify asset exists in this plant and org before generating RCA
		var assetExists bool
		checkErr := dbPool.QueryRow(c.Request.Context(), `
			SELECT EXISTS (
				SELECT 1 FROM asset.assets
				WHERE UPPER(asset_tag) = UPPER($1)
				  AND organization_id = $2
				  AND plant_id = $3
			)
		`, req.AssetTag, req.OrganizationID, req.PlantID).Scan(&assetExists)
		if checkErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to verify asset"})
			return
		}
		if !assetExists {
			c.JSON(http.StatusNotFound, gin.H{"error": "asset not found in the specified plant"})
			return
		}

		payloadBytes, err := json.Marshal(req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to marshal request: %v", err)})
			return
		}

		recordAuditEvent(c, dbPool, req.PlantID, "RCA_GENERATION_REQUESTED", "asset", req.AssetTag, map[string]interface{}{
			"failureDescriptionChars": len(req.FailureDescription),
		})

		aiResp, err := callAIService(c.Request.Context(), http.MethodPost, aiServiceURL, "/rca", payloadBytes)
		if err != nil || aiResp.StatusCode < 200 || aiResp.StatusCode >= 300 {
			reason := aiFailureReason(err, aiResp)
			recordAuditEvent(c, dbPool, req.PlantID, "RCA_GENERATION_FALLBACK", "asset", req.AssetTag, map[string]interface{}{
				"reason": reason,
			})
			c.JSON(http.StatusOK, fallbackRCAResponse(req.AssetTag, reason))
			return
		}

		c.Data(aiResp.StatusCode, "application/json", aiResp.Body)
	}
}

func fallbackRCAResponse(assetTag, reason string) gin.H {
	missingData := []string{"AI service unavailable or timed out; RCA could not be generated from evidence"}
	if strings.TrimSpace(reason) != "" {
		missingData = append(missingData, reason)
	}
	return gin.H{
		"summary":         fmt.Sprintf("RCA for %s could not be generated safely because the AI service is unavailable.", strings.ToUpper(strings.TrimSpace(assetTag))),
		"probableCauses":  []string{"Data unavailable"},
		"recommendations": []string{"Retry RCA generation after AI processing is healthy", "Review available work orders and inspection records manually before taking action"},
		"confidence":      0.0,
		"citations":       []gin.H{},
		"missingData":     missingData,
		"fallback":        true,
	}
}

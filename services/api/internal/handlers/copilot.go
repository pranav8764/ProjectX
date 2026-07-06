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

// HandleCopilotQuery handles /api/copilot/query and routes it to the AI Service.
func HandleCopilotQuery(dbPool *pgxpool.Pool, aiServiceURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.QueryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Inject plantID from context if missing
		if req.PlantID == "" {
			req.PlantID = c.GetString("plantID")
		}
		req.UserID = c.GetString("userID")
		req.OrganizationID = c.GetString("orgID")
		req.UserRole = auth.NormalizeRole(c.GetString("role"))
		req.AccessContext = auth.DocumentAccessContextForRole(c.GetString("role"))
		if req.Filters == nil {
			req.Filters = map[string]interface{}{}
		}
		req.Filters["documentAccess"] = req.AccessContext

		if req.PlantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "plantId is required"})
			return
		}
		if !auth.RequirePlantAccess(c, dbPool, req.PlantID) {
			return
		}

		payloadBytes, err := json.Marshal(req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to marshal request: %v", err)})
			return
		}

		recordAuditEvent(c, dbPool, req.PlantID, "COPILOT_QUERY_REQUESTED", "copilot_query", "", map[string]interface{}{
			"questionChars": len(req.Question),
			"hasFilters":    len(req.Filters) > 0,
		})

		aiResp, err := callAIService(c.Request.Context(), http.MethodPost, aiServiceURL, "/query", payloadBytes)
		if err != nil || aiResp.StatusCode < 200 || aiResp.StatusCode >= 300 {
			reason := aiFailureReason(err, aiResp)
			recordAuditEvent(c, dbPool, req.PlantID, "COPILOT_QUERY_FALLBACK", "copilot_query", "", map[string]interface{}{
				"reason": reason,
			})
			c.JSON(http.StatusOK, fallbackCopilotResponse(req.Question, reason))
			return
		}

		c.Data(aiResp.StatusCode, "application/json", aiResp.Body)
	}
}

func fallbackCopilotResponse(question, reason string) gin.H {
	missingInfo := []string{"AI service unavailable or timed out; retry the query after processing services recover"}
	if strings.TrimSpace(reason) != "" {
		missingInfo = append(missingInfo, reason)
	}
	return gin.H{
		"answer":        "Not enough evidence is available to answer safely right now because the AI service could not complete the request.",
		"confidence":    0.0,
		"citations":     []gin.H{},
		"relatedAssets": extractAssetTags(question),
		"missingInfo":   missingInfo,
		"fallback":      true,
	}
}

func extractAssetTags(text string) []string {
	fields := strings.FieldsFunc(strings.ToUpper(text), func(r rune) bool {
		return !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-'
	})

	seen := map[string]bool{}
	tags := []string{}
	for _, field := range fields {
		field = strings.Trim(field, "-")
		if field == "" || seen[field] || !looksLikeAssetTag(field) {
			continue
		}
		seen[field] = true
		tags = append(tags, field)
	}
	return tags
}

func looksLikeAssetTag(value string) bool {
	hasLetter := false
	hasDigit := false
	hasSeparator := strings.Contains(value, "-")
	for _, r := range value {
		if r >= 'A' && r <= 'Z' {
			hasLetter = true
		}
		if r >= '0' && r <= '9' {
			hasDigit = true
		}
	}
	return hasLetter && hasDigit && hasSeparator
}

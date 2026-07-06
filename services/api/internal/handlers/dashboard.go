package handlers

import (
	"fmt"
	"net/http"

	"plantbrain-api/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HandleDashboardMetrics aggregates system metrics across the workspace or plant.
func HandleDashboardMetrics(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("orgID")
		plantID := c.Query("plantId")
		if plantID == "" {
			plantID = c.GetString("plantID")
		}
		if plantID != "" && !auth.RequirePlantAccess(c, dbPool, plantID) {
			return
		}

		var totalDocuments, processedDocuments, failedDocuments int
		var assetsDiscovered, criticalAssets, rcaReports, openComplianceGaps int
		var totalQueries, citedQueries, graphEntities, graphRelationships int
		var avgQueryConfidence float64

		err := dbPool.QueryRow(c.Request.Context(), `
			SELECT
				(SELECT COUNT(*) FROM document.documents WHERE organization_id = $1 AND status <> 'ARCHIVED' AND ($2::uuid IS NULL OR plant_id = $2::uuid)),
				(SELECT COUNT(*) FROM document.documents WHERE organization_id = $1 AND status = 'COMPLETED' AND ($2::uuid IS NULL OR plant_id = $2::uuid)),
				(SELECT COUNT(*) FROM document.documents WHERE organization_id = $1 AND status = 'FAILED' AND ($2::uuid IS NULL OR plant_id = $2::uuid)),
				(SELECT COUNT(*) FROM asset.assets WHERE organization_id = $1 AND ($2::uuid IS NULL OR plant_id = $2::uuid)),
				(SELECT COUNT(*) FROM asset.assets WHERE organization_id = $1 AND ($2::uuid IS NULL OR plant_id = $2::uuid) AND UPPER(COALESCE(criticality, '')) IN ('CRITICAL', 'HIGH')),
				(SELECT COUNT(*) FROM rca.reports r LEFT JOIN asset.assets a ON r.asset_id = a.id WHERE r.organization_id = $1 AND ($2::uuid IS NULL OR r.plant_id = $2::uuid OR a.plant_id = $2::uuid)),
				(SELECT COUNT(*) FROM compliance.gaps WHERE organization_id = $1 AND status = 'OPEN' AND ($2::uuid IS NULL OR plant_id = $2::uuid)),
				(SELECT COUNT(*) FROM rag.queries WHERE organization_id = $1 AND ($2::uuid IS NULL OR plant_id = $2::uuid)),
				(SELECT COUNT(DISTINCT q.id) FROM rag.queries q JOIN rag.citations c ON c.query_id = q.id WHERE q.organization_id = $1 AND ($2::uuid IS NULL OR q.plant_id = $2::uuid)),
				(SELECT COUNT(*) FROM graph.entities WHERE organization_id = $1 AND ($2::uuid IS NULL OR plant_id = $2::uuid)),
				(SELECT COUNT(*) FROM graph.relationships r JOIN graph.entities e ON r.source_entity_id = e.id WHERE r.organization_id = $1 AND ($2::uuid IS NULL OR e.plant_id = $2::uuid)),
				(SELECT COALESCE(AVG(confidence), 0) FROM rag.queries WHERE organization_id = $1 AND ($2::uuid IS NULL OR plant_id = $2::uuid))
		`, orgID, nullableUUID(plantID)).Scan(
			&totalDocuments, &processedDocuments, &failedDocuments,
			&assetsDiscovered, &criticalAssets, &rcaReports, &openComplianceGaps,
			&totalQueries, &citedQueries, &graphEntities, &graphRelationships, &avgQueryConfidence,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to load dashboard metrics: %v", err)})
			return
		}

		statusRows, err := dbPool.Query(c.Request.Context(), `
			SELECT status, COUNT(*)
			FROM document.documents
			WHERE organization_id = $1
			  AND status <> 'ARCHIVED'
			  AND ($2::uuid IS NULL OR plant_id = $2::uuid)
			GROUP BY status
			ORDER BY status
		`, orgID, nullableUUID(plantID))
		statusCounts := gin.H{}
		if err == nil {
			defer statusRows.Close()
			for statusRows.Next() {
				var status string
				var count int
				if err := statusRows.Scan(&status, &count); err == nil {
					statusCounts[status] = count
				}
			}
		}

		querySuccessRate := 0.0
		if totalQueries > 0 {
			querySuccessRate = float64(citedQueries) / float64(totalQueries)
		}

		graphCompleteness := 0.0
		if graphEntities > 0 {
			graphCompleteness = float64(graphRelationships) / float64(graphEntities)
			if graphCompleteness > 1 {
				graphCompleteness = 1
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"plantId":                     plantID,
			"totalDocuments":              totalDocuments,
			"documentsProcessed":          processedDocuments,
			"documentsFailed":             failedDocuments,
			"assetsDiscovered":            assetsDiscovered,
			"criticalAssets":              criticalAssets,
			"repeatedFailures":            rcaReports,
			"complianceGaps":              openComplianceGaps,
			"querySuccessRate":            querySuccessRate,
			"averageQueryConfidence":      avgQueryConfidence,
			"knowledgeGraphEntities":      graphEntities,
			"knowledgeGraphRelationships": graphRelationships,
			"knowledgeGraphCompleteness":  graphCompleteness,
			"timeSavedEstimateMinutes":    citedQueries * 20,
			"documentProcessingStatus":    statusCounts,
		})
	}
}

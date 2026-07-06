package handlers

import (
	"fmt"
	"net/http"

	"plantbrain-api/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HandleGetGraph retrieves knowledge graph nodes and edges with access control filters.
func HandleGetGraph(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("orgID")
		plantID := c.Query("plantId")
		if plantID == "" {
			plantID = c.GetString("plantID")
		}
		if plantID != "" && !auth.RequirePlantAccess(c, dbPool, plantID) {
			return
		}
		sourcePolicy := auth.DocumentSourcePolicyForRole(c.GetString("role"))

		nodes := []gin.H{}
		nodeRows, err := dbPool.Query(c.Request.Context(), `
			SELECT e.id::text, e.entity_type, e.entity_value, COALESCE(e.normalized_value, ''), e.confidence, e.document_id::text, e.page_no
			FROM graph.entities e
			LEFT JOIN document.documents d ON e.document_id = d.id
			WHERE e.organization_id = $1
			  AND ($2::uuid IS NULL OR e.plant_id = $2::uuid)
			  AND (
			    e.document_id IS NULL
			    OR COALESCE(d.access_level, 'internal') = ANY($3)
			    OR COALESCE(d.allowed_roles, ARRAY[]::text[]) && $4
			  )
			ORDER BY e.created_at DESC
			LIMIT 300
		`, orgID, nullableUUID(plantID), sourcePolicy.ReadableAccessLevels, sourcePolicy.AllowedRoles)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to load graph nodes: %v", err)})
			return
		}
		defer nodeRows.Close()
		for nodeRows.Next() {
			var id, entityType, entityValue, normalized string
			var confidence *float64
			var documentID *string
			var pageNo *int
			if err := nodeRows.Scan(&id, &entityType, &entityValue, &normalized, &confidence, &documentID, &pageNo); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to scan graph node: %v", err)})
				return
			}
			nodes = append(nodes, gin.H{
				"id":              id,
				"type":            entityType,
				"label":           entityValue,
				"normalizedValue": normalized,
				"confidence":      floatValue(confidence),
				"documentId":      documentID,
				"pageNo":          pageNo,
			})
		}

		edges := []gin.H{}
		edgeRows, err := dbPool.Query(c.Request.Context(), `
			SELECT
				r.id::text,
				r.source_entity_id::text,
				r.target_entity_id::text,
				r.relationship_type,
				r.confidence,
				r.evidence_document_id::text,
				r.evidence_chunk_id::text
			FROM graph.relationships r
			JOIN graph.entities s ON r.source_entity_id = s.id
			JOIN graph.entities t ON r.target_entity_id = t.id
			LEFT JOIN document.documents d ON r.evidence_document_id = d.id
			WHERE r.organization_id = $1
			  AND ($2::uuid IS NULL OR s.plant_id = $2::uuid OR t.plant_id = $2::uuid)
			  AND (
			    r.evidence_document_id IS NULL
			    OR COALESCE(d.access_level, 'internal') = ANY($3)
			    OR COALESCE(d.allowed_roles, ARRAY[]::text[]) && $4
			  )
			ORDER BY r.created_at DESC
			LIMIT 500
		`, orgID, nullableUUID(plantID), sourcePolicy.ReadableAccessLevels, sourcePolicy.AllowedRoles)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to load graph edges: %v", err)})
			return
		}
		defer edgeRows.Close()
		for edgeRows.Next() {
			var id, sourceID, targetID, relType string
			var confidence *float64
			var evidenceDocumentID, evidenceChunkID *string
			if err := edgeRows.Scan(&id, &sourceID, &targetID, &relType, &confidence, &evidenceDocumentID, &evidenceChunkID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to scan graph edge: %v", err)})
				return
			}
			edges = append(edges, gin.H{
				"id":                 id,
				"source":             sourceID,
				"target":             targetID,
				"type":               relType,
				"confidence":         floatValue(confidence),
				"evidenceDocumentId": evidenceDocumentID,
				"evidenceChunkId":    evidenceChunkID,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"plantId": plantID,
			"nodes":   nodes,
			"edges":   edges,
		})
	}
}

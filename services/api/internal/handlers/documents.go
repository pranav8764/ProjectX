package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"plantbrain-api/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
)

const processingRetryBaseDelay = time.Minute
const processingRetryMaxDelay = 30 * time.Minute

// HandleGetDocuments retrieves a list of uploaded documents.
func HandleGetDocuments(dbPool *pgxpool.Pool) gin.HandlerFunc {
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
		role := auth.NormalizeRole(c.GetString("role"))
		rows, err := dbPool.Query(ctx, `
			SELECT
				d.id::text,
				d.title,
				COALESCE(v.file_type, ''),
				COALESCE(d.document_type, ''),
				COALESCE(d.access_level, 'internal'),
				COALESCE(d.sensitivity, 'standard'),
				COALESCE(d.allowed_roles, ARRAY[]::text[]),
				d.status,
				COALESCE(v.ocr_confidence, 0),
				COALESCE(v.classification_confidence, 0),
				d.created_at
			FROM document.documents d
			LEFT JOIN document.document_versions v ON d.current_version_id = v.id
			WHERE d.organization_id = $1
			  AND ($2::uuid IS NULL OR d.plant_id = $2::uuid)
			  AND d.status <> 'ARCHIVED'
			ORDER BY d.created_at DESC
		`, orgID, nullableUUID(plantID))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to retrieve documents: %v", err)})
			return
		}
		defer rows.Close()

		documents := []gin.H{}
		for rows.Next() {
			var id, title, fileType, documentType, accessLevel, sensitivity, status string
			var allowedRoles []string
			var ocrConfidence, classificationConfidence float64
			var createdAt time.Time
			if err := rows.Scan(&id, &title, &fileType, &documentType, &accessLevel, &sensitivity, &allowedRoles, &status, &ocrConfidence, &classificationConfidence, &createdAt); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to scan document: %v", err)})
				return
			}
			documents = append(documents, gin.H{
				"id":                       id,
				"title":                    title,
				"fileType":                 strings.ToUpper(fileType),
				"documentType":             documentType,
				"accessLevel":              accessLevel,
				"sensitivity":              sensitivity,
				"allowedRoles":             allowedRoles,
				"sourceRestricted":         auth.DocumentSourceRestricted(accessLevel, allowedRoles),
				"sourceDownloadAllowed":    auth.DocumentReadableByRole(role, accessLevel, allowedRoles),
				"status":                   status,
				"ocrConfidence":            ocrConfidence,
				"classificationConfidence": classificationConfidence,
				"createdAt":                createdAt,
			})
		}

		c.JSON(http.StatusOK, documents)
	}
}

// HandleGetDocumentDetail returns detailed metadata, pages, chunks, and entities of a document.
func HandleGetDocumentDetail(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		docID := c.Param("id")
		orgID := c.GetString("orgID")
		ctx := c.Request.Context()

		var documentID, organizationID, plantID, title, documentType, accessLevel, sensitivity, status string
		var allowedRoles []string
		var uploadedBy *string
		var createdAt, updatedAt time.Time
		var versionID, versionLabel, fileURL, fileType, fileSHA, markdownContent *string
		var ocrConfidence, classificationConfidence *float64
		var versionCreatedAt *time.Time

		err := dbPool.QueryRow(ctx, `
			SELECT
				d.id::text,
				d.organization_id::text,
				d.plant_id::text,
				d.title,
				COALESCE(d.document_type, ''),
				COALESCE(d.access_level, 'internal'),
				COALESCE(d.sensitivity, 'standard'),
				COALESCE(d.allowed_roles, ARRAY[]::text[]),
				d.status,
				d.uploaded_by::text,
				d.created_at,
				d.updated_at,
				v.id::text,
				v.version_label,
				v.file_url,
				v.file_type,
				v.file_sha256,
				v.markdown_content,
				v.ocr_confidence,
				v.classification_confidence,
				v.created_at
			FROM document.documents d
			LEFT JOIN document.document_versions v ON d.current_version_id = v.id
			WHERE d.id = $1 AND d.organization_id = $2
		`, docID, orgID).Scan(
			&documentID, &organizationID, &plantID, &title, &documentType, &accessLevel, &sensitivity, &allowedRoles, &status, &uploadedBy, &createdAt, &updatedAt,
			&versionID, &versionLabel, &fileURL, &fileType, &fileSHA, &markdownContent, &ocrConfidence, &classificationConfidence, &versionCreatedAt,
		)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
			return
		}
		if !auth.RequirePlantAccess(c, dbPool, plantID) {
			return
		}
		sourceDownloadAllowed := auth.DocumentReadableByRole(c.GetString("role"), accessLevel, allowedRoles)

		pages := []gin.H{}
		if sourceDownloadAllowed {
			pageRows, err := dbPool.Query(ctx, `
				SELECT page_no, COALESCE(raw_text, ''), COALESCE(markdown_text, ''), COALESCE(ocr_confidence, 0), metadata_json
				FROM ingestion.document_pages
				WHERE document_id = $1
				  AND ($2::uuid IS NULL OR document_version_id = $2::uuid)
				ORDER BY page_no
			`, docID, versionID)
			if err == nil {
				defer pageRows.Close()
				for pageRows.Next() {
					var pageNo int
					var rawText, markdownText string
					var confidence float64
					var metadata []byte
					if err := pageRows.Scan(&pageNo, &rawText, &markdownText, &confidence, &metadata); err == nil {
						pages = append(pages, gin.H{
							"pageNo":        pageNo,
							"rawText":       rawText,
							"markdownText":  markdownText,
							"ocrConfidence": confidence,
							"metadata":      json.RawMessage(metadata),
						})
					}
				}
			}
		}

		chunks := []gin.H{}
		if sourceDownloadAllowed {
			chunkRows, err := dbPool.Query(ctx, `
				SELECT id::text, page_no, chunk_index, chunk_text, COALESCE(token_count, 0), metadata_json
				FROM ingestion.document_chunks
				WHERE document_id = $1
				  AND ($2::uuid IS NULL OR document_version_id = $2::uuid)
				ORDER BY chunk_index
				LIMIT 50
			`, docID, versionID)
			if err == nil {
				defer chunkRows.Close()
				for chunkRows.Next() {
					var id, text string
					var pageNo *int
					var chunkIndex, tokenCount int
					var metadata []byte
					if err := chunkRows.Scan(&id, &pageNo, &chunkIndex, &text, &tokenCount, &metadata); err == nil {
						chunks = append(chunks, gin.H{
							"id":         id,
							"pageNo":     pageNo,
							"chunkIndex": chunkIndex,
							"text":       text,
							"tokenCount": tokenCount,
							"metadata":   json.RawMessage(metadata),
						})
					}
				}
			}
		}

		entities := []gin.H{}
		if sourceDownloadAllowed {
			entityRows, err := dbPool.Query(ctx, `
				SELECT e.id::text, e.entity_type, e.entity_value, COALESCE(e.normalized_value, ''), COALESCE(e.confidence, 0), e.page_no
				FROM graph.entities e
				LEFT JOIN ingestion.document_chunks c ON e.chunk_id = c.id
				WHERE e.document_id = $1
				  AND ($2::uuid IS NULL OR e.chunk_id IS NULL OR c.document_version_id = $2::uuid)
				ORDER BY e.confidence DESC, e.entity_type, e.entity_value
				LIMIT 100
			`, docID, versionID)
			if err == nil {
				defer entityRows.Close()
				for entityRows.Next() {
					var id, entityType, entityValue, normalized string
					var confidence float64
					var pageNo *int
					if err := entityRows.Scan(&id, &entityType, &entityValue, &normalized, &confidence, &pageNo); err == nil {
						entities = append(entities, gin.H{
							"id":              id,
							"type":            entityType,
							"value":           entityValue,
							"normalizedValue": normalized,
							"confidence":      confidence,
							"pageNo":          pageNo,
						})
					}
				}
			}
		}

		recordAuditEvent(c, dbPool, plantID, "DOCUMENT_VIEWED", "document", docID, map[string]interface{}{
			"status":       status,
			"documentType": documentType,
			"accessLevel":  accessLevel,
			"sensitivity":  sensitivity,
			"sourceAccess": sourceDownloadAllowed,
		})

		version := gin.H{}
		if versionID != nil {
			version = gin.H{
				"id":                       *versionID,
				"versionLabel":             stringValue(versionLabel),
				"fileType":                 stringValue(fileType),
				"ocrConfidence":            floatValue(ocrConfidence),
				"classificationConfidence": floatValue(classificationConfidence),
				"createdAt":                versionCreatedAt,
			}
			if sourceDownloadAllowed {
				version["fileUrl"] = stringValue(fileURL)
				version["fileSha256"] = stringValue(fileSHA)
				version["markdownContent"] = stringValue(markdownContent)
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"id":                    documentID,
			"organizationId":        organizationID,
			"plantId":               plantID,
			"title":                 title,
			"documentType":          documentType,
			"accessLevel":           accessLevel,
			"sensitivity":           sensitivity,
			"allowedRoles":          allowedRoles,
			"sourceRestricted":      auth.DocumentSourceRestricted(accessLevel, allowedRoles),
			"sourceDownloadAllowed": sourceDownloadAllowed,
			"status":                status,
			"uploadedBy":            uploadedBy,
			"createdAt":             createdAt,
			"updatedAt":             updatedAt,
			"currentVersion":        version,
			"pages":                 pages,
			"chunks":                chunks,
			"entities":              entities,
		})
	}
}

// HandleGetDocumentStatus returns the status details of a document's background processing job.
func HandleGetDocumentStatus(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		docID := c.Param("id")
		orgID := c.GetString("orgID")

		var plantID, accessLevel, documentStatus string
		var allowedRoles []string
		var updatedAt time.Time
		var versionID, jobID, jobStatus, errorMessage, lockedBy *string
		var progress *float64
		var attempts *int
		var startedAt, completedAt, jobUpdatedAt, lastAttemptedAt, nextRetryAt, lockedAt *time.Time
		var metadataJSONBytes []byte

		err := dbPool.QueryRow(c.Request.Context(), `
			SELECT
				d.plant_id::text,
				COALESCE(d.access_level, 'internal'),
				COALESCE(d.allowed_roles, ARRAY[]::text[]),
				d.status,
				d.updated_at,
				d.current_version_id::text,
				j.id::text,
				j.status,
				j.error_message,
				j.progress,
				j.attempts,
				j.last_attempted_at,
				j.next_retry_at,
				j.locked_at,
				j.locked_by,
				j.started_at,
				j.completed_at,
				j.updated_at,
				j.metadata_json
			FROM document.documents d
			LEFT JOIN LATERAL (
				SELECT id, status, error_message, progress, attempts, last_attempted_at, next_retry_at, locked_at, locked_by, started_at, completed_at, updated_at, metadata_json
				FROM ingestion.processing_jobs
				WHERE document_id = d.id
				ORDER BY created_at DESC
				LIMIT 1
			) j ON TRUE
			WHERE d.id = $1 AND d.organization_id = $2
		`, docID, orgID).Scan(&plantID, &accessLevel, &allowedRoles, &documentStatus, &updatedAt, &versionID, &jobID, &jobStatus, &errorMessage, &progress, &attempts, &lastAttemptedAt, &nextRetryAt, &lockedAt, &lockedBy, &startedAt, &completedAt, &jobUpdatedAt, &metadataJSONBytes)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
			return
		}
		if !auth.RequirePlantAccess(c, dbPool, plantID) {
			return
		}

		processingAttempts := 0
		if attempts != nil {
			processingAttempts = *attempts
		}
		processingStatus := documentStatus
		if jobStatus != nil && *jobStatus != "" {
			processingStatus = *jobStatus
		}
		retryAvailable := strings.EqualFold(processingStatus, "FAILED") || strings.EqualFold(documentStatus, "FAILED")
		retryURL := ""
		if retryAvailable {
			retryURL = fmt.Sprintf("/api/documents/%s/retry-processing", docID)
		}

		var errorMetadata map[string]interface{}
		if len(metadataJSONBytes) > 0 {
			json.Unmarshal(metadataJSONBytes, &errorMetadata)
		}

		c.JSON(http.StatusOK, gin.H{
			"documentId":         docID,
			"plantId":            plantID,
			"status":             documentStatus,
			"currentVersionId":   versionID,
			"updatedAt":          updatedAt,
			"processingJobId":    jobID,
			"processingStatus":   processingStatus,
			"processingProgress": floatValue(progress),
			"processingAttempts": processingAttempts,
			"attempts":           processingAttempts,
			"retryAvailable":     retryAvailable,
			"retryUrl":           retryURL,
			"errorMessage":       errorMessage,
			"lastAttemptedAt":    lastAttemptedAt,
			"nextRetryAt":        nextRetryAt,
			"lockedAt":           lockedAt,
			"lockedBy":           lockedBy,
			"startedAt":          startedAt,
			"completedAt":        completedAt,
			"jobUpdatedAt":       jobUpdatedAt,
			"errorMetadata":      errorMetadata,
		})
	}
}

// HandleGetDocumentDownloadURL generates a signed URL from MinIO for temporary download access.
func HandleGetDocumentDownloadURL(dbPool *pgxpool.Pool, minioClient *minio.Client, bucketName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		docID := c.Param("id")
		orgID := c.GetString("orgID")
		ctx := c.Request.Context()

		var plantID, title, accessLevel, sensitivity, status string
		var allowedRoles []string
		var objectKey *string
		var fileType *string
		err := dbPool.QueryRow(ctx, `
			SELECT
				d.plant_id::text,
				d.title,
				COALESCE(d.access_level, 'internal'),
				COALESCE(d.sensitivity, 'standard'),
				COALESCE(d.allowed_roles, ARRAY[]::text[]),
				d.status,
				u.object_key,
				v.file_type
			FROM document.documents d
			LEFT JOIN LATERAL (
				SELECT object_key
				FROM document.upload_sessions
				WHERE document_id = d.id
				  AND organization_id = d.organization_id
				  AND status = 'COMPLETED'
				ORDER BY completed_at DESC NULLS LAST, created_at DESC
				LIMIT 1
			) u ON TRUE
			LEFT JOIN document.document_versions v ON d.current_version_id = v.id
			WHERE d.id = $1 AND d.organization_id = $2
		`, docID, orgID).Scan(&plantID, &title, &accessLevel, &sensitivity, &allowedRoles, &status, &objectKey, &fileType)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
			return
		}
		if status == "ARCHIVED" {
			c.JSON(http.StatusGone, gin.H{"error": "document is archived"})
			return
		}
		if !auth.RequirePlantAccess(c, dbPool, plantID) {
			return
		}
		if !auth.RequireDocumentAccess(c, accessLevel, allowedRoles) {
			return
		}
		if objectKey == nil || *objectKey == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "document file object not found"})
			return
		}

		params := make(url.Values)
		params.Set("response-content-disposition", fmt.Sprintf("attachment; filename=%q", safeDownloadFileName(title, stringValue(fileType))))

		signedURL, err := minioClient.PresignedGetObject(ctx, bucketName, *objectKey, 10*time.Minute, params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create signed download URL: %v", err)})
			return
		}

		recordAuditEvent(c, dbPool, plantID, "DOCUMENT_DOWNLOAD_LINK_CREATED", "document", docID, map[string]interface{}{
			"objectKey":        *objectKey,
			"expiresInSeconds": 600,
		})

		c.JSON(http.StatusOK, gin.H{
			"documentId":   docID,
			"accessLevel":  accessLevel,
			"sensitivity":  sensitivity,
			"downloadUrl":  signedURL.String(),
			"expiresAt":    time.Now().Add(10 * time.Minute),
			"expiresInSec": 600,
		})
	}
}

// HandleArchiveDocument soft deletes a document and cleans up all derived vector and graph data.
func HandleArchiveDocument(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		docID := c.Param("id")
		orgID := c.GetString("orgID")
		userID := c.GetString("userID")
		ctx := c.Request.Context()

		var plantID, accessLevel string
		var allowedRoles []string
		err := dbPool.QueryRow(ctx, `
			SELECT plant_id::text, COALESCE(access_level, 'internal'), COALESCE(allowed_roles, ARRAY[]::text[])
			FROM document.documents
			WHERE id = $1 AND organization_id = $2
		`, docID, orgID).Scan(&plantID, &accessLevel, &allowedRoles)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
			return
		}
		if !auth.RequirePlantAccess(c, dbPool, plantID) {
			return
		}
		if !auth.RequireDocumentAccess(c, accessLevel, allowedRoles) {
			return
		}

		tx, err := dbPool.Begin(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to start archive transaction: %v", err)})
			return
		}
		defer tx.Rollback(ctx)

		if _, err = tx.Exec(ctx, `
			UPDATE document.documents
			SET status = 'ARCHIVED', updated_at = NOW()
			WHERE id = $1 AND organization_id = $2
		`, docID, orgID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to archive document: %v", err)})
			return
		}

		if _, err = tx.Exec(ctx, `
			DELETE FROM graph.relationships r
			USING graph.entities e
			WHERE (r.source_entity_id = e.id OR r.target_entity_id = e.id)
			  AND e.document_id = $1
			  AND r.organization_id = $2
		`, docID, orgID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to remove graph relationships: %v", err)})
			return
		}

		cleanupStatements := []string{
			`DELETE FROM graph.relationships WHERE evidence_document_id = $1 AND organization_id = $2`,
			`DELETE FROM graph.entities WHERE document_id = $1 AND organization_id = $2`,
			`DELETE FROM rag.citations WHERE document_id = $1`,
			`DELETE FROM ingestion.document_chunks WHERE document_id = $1`,
			`DELETE FROM ingestion.document_pages WHERE document_id = $1`,
			`UPDATE ingestion.processing_jobs SET status = 'ARCHIVED', progress = 1, completed_at = COALESCE(completed_at, NOW()), updated_at = NOW() WHERE document_id = $1`,
		}
		for _, stmt := range cleanupStatements {
			if strings.Contains(stmt, "$2") {
				_, err = tx.Exec(ctx, stmt, docID, orgID)
			} else {
				_, err = tx.Exec(ctx, stmt, docID)
			}
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to remove derived document data: %v", err)})
				return
			}
		}

		if _, err = tx.Exec(ctx, `
			INSERT INTO audit.events (organization_id, plant_id, actor_user_id, event_type, resource_type, resource_id, correlation_id, metadata_json)
			VALUES ($1, $2, $3, 'DOCUMENT_ARCHIVED', 'document', $4, $5, $6)
		`, orgID, plantID, userID, docID, c.GetString("requestID"), auditMetadataJSON(c, map[string]interface{}{
			"derivedArtifactsRemoved": true,
		})); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to record archive audit event: %v", err)})
			return
		}

		if err = tx.Commit(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to commit archive: %v", err)})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"documentId": docID,
			"status":     "ARCHIVED",
			"message":    "Document archived and derived graph/search artifacts removed",
		})
	}
}

// HandleDocumentUpload writes the file locally, uploads to MinIO, records metadata, and queues AI ingestion.
func HandleDocumentUpload(dbPool *pgxpool.Pool, minioClient *minio.Client, bucketName, aiServiceURL, s3Endpoint string) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file field is required"})
			return
		}
		defer file.Close()

		plantID := c.PostForm("plantId")
		if plantID == "" {
			plantID = c.GetString("plantID")
		}

		documentType := c.PostForm("documentType")
		if documentType == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "documentType field is required"})
			return
		}

		orgID := c.GetString("orgID")
		userID := c.GetString("userID")
		accessPolicy, err := auth.BuildDocumentAccessPolicy(
			documentType,
			c.PostForm("accessLevel"),
			c.PostForm("sensitivity"),
			c.PostForm("allowedRoles"),
			c.GetString("role"),
		)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "cannot set document access policy") {
				status = http.StatusForbidden
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}

		if plantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "plantId field is required"})
			return
		}
		if !auth.RequirePlantAccess(c, dbPool, plantID) {
			return
		}

		uploadsDir := os.Getenv("UPLOADS_DIR")
		if uploadsDir == "" {
			uploadsDir = "/app/uploads"
		}
		if err := os.MkdirAll(uploadsDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create upload directory: %v", err)})
			return
		}

		docID := uuid.New()
		versionID := uuid.New()

		const maxUploadSize = 50 * 1024 * 1024 // 50 MB
		if header.Size > maxUploadSize {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file size exceeds 50MB limit"})
			return
		}

		ext := filepath.Ext(header.Filename)
		if !allowedDocumentExtension(ext) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported file type"})
			return
		}

		if strings.ToLower(ext) == ".pdf" {
			buf := make([]byte, 5)
			if _, err := io.ReadFull(file, buf); err != nil || string(buf) != "%PDF-" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "corrupt PDF file"})
				return
			}
			file.Seek(0, io.SeekStart)
		}

		localFileName := fmt.Sprintf("%s%s", docID.String(), ext)
		localFilePath := filepath.Join(uploadsDir, localFileName)

		out, err := os.Create(localFilePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create local file: %v", err)})
			return
		}

		hasher := sha256.New()
		writer := io.MultiWriter(out, hasher)
		if _, err := io.Copy(writer, file); err != nil {
			out.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to save local file: %v", err)})
			return
		}
		out.Close()

		fileSHA256 := hex.EncodeToString(hasher.Sum(nil))

		localStat, err := os.Stat(localFilePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to inspect uploaded file: %v", err)})
			return
		}
		if localStat.Size() == 0 {
			_ = os.Remove(localFilePath)
			c.JSON(http.StatusBadRequest, gin.H{"error": "uploaded file is empty"})
			return
		}

		var duplicateDocID string
		var duplicateStatus string
		err = dbPool.QueryRow(c.Request.Context(), `
			SELECT d.id::text, d.status
			FROM document.document_versions v
			JOIN document.documents d ON v.document_id = d.id
			WHERE d.organization_id = $1
			  AND d.plant_id = $2
			  AND v.file_sha256 = $3
			  AND d.status <> 'ARCHIVED'
			ORDER BY d.created_at DESC
			LIMIT 1
		`, orgID, plantID, fileSHA256).Scan(&duplicateDocID, &duplicateStatus)
		if err == nil {
			_ = os.Remove(localFilePath)
			recordAuditEvent(c, dbPool, plantID, "DOCUMENT_UPLOAD_DUPLICATE_DETECTED", "document", duplicateDocID, map[string]interface{}{
				"fileName":   header.Filename,
				"fileSha256": fileSHA256,
			})
			c.JSON(http.StatusOK, gin.H{
				"documentId": duplicateDocID,
				"status":     duplicateStatus,
				"duplicate":  true,
				"message":    "Document already exists for this plant",
			})
			return
		}
		if err != pgx.ErrNoRows {
			_ = os.Remove(localFilePath)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to check duplicate upload: %v", err)})
			return
		}

		minioFile, err := os.Open(localFilePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to open file for MinIO upload: %v", err)})
			return
		}
		defer minioFile.Close()

		fileStat, err := minioFile.Stat()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to stat file for MinIO upload: %v", err)})
			return
		}

		objectKey := fmt.Sprintf("uploads/%s/%s", docID.String(), filepath.Base(header.Filename))
		_, err = minioClient.PutObject(c.Request.Context(), bucketName, objectKey, minioFile, fileStat.Size(), minio.PutObjectOptions{
			ContentType: header.Header.Get("Content-Type"),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to upload to MinIO: %v", err)})
			return
		}

		fileURL := fmt.Sprintf("%s/%s/%s", strings.TrimRight(s3Endpoint, "/"), bucketName, objectKey)

		ctx := c.Request.Context()
		tx, err := dbPool.Begin(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to start database transaction: %v", err)})
			return
		}
		defer tx.Rollback(ctx)

		_, err = tx.Exec(ctx, `
			INSERT INTO document.documents (
				id, organization_id, plant_id, title, document_type, access_level, sensitivity, allowed_roles,
				current_version_id, status, uploaded_by, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULL, 'UPLOADED', $9, NOW(), NOW())
		`, docID, orgID, plantID, header.Filename, documentType, accessPolicy.AccessLevel, accessPolicy.Sensitivity, accessPolicy.AllowedRoles, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to insert document metadata: %v", err)})
			return
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO document.document_versions (id, document_id, version_label, file_url, file_type, file_sha256, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, NOW())
		`, versionID, docID, "v1", fileURL, strings.TrimPrefix(ext, "."), fileSHA256)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to insert document version: %v", err)})
			return
		}

		_, err = tx.Exec(ctx, `
			UPDATE document.documents
			SET current_version_id = $1
			WHERE id = $2
		`, versionID, docID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to link document version: %v", err)})
			return
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO document.upload_sessions (document_id, organization_id, plant_id, object_key, status, created_by, created_at, completed_at)
			VALUES ($1, $2, $3, $4, 'COMPLETED', $5, NOW(), NOW())
		`, docID, orgID, plantID, objectKey, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to record upload session: %v", err)})
			return
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO ingestion.processing_jobs (
				document_id, document_version_id, status, attempts, progress,
				next_retry_at, metadata_json, created_at, updated_at
			)
			VALUES ($1, $2, 'QUEUED', 0, 0, NOW(), $3, NOW(), NOW())
			ON CONFLICT (document_id, document_version_id)
			DO UPDATE SET
				status = 'QUEUED',
				error_message = NULL,
				progress = 0,
				next_retry_at = NOW(),
				locked_at = NULL,
				locked_by = NULL,
				metadata_json = ingestion.processing_jobs.metadata_json || EXCLUDED.metadata_json,
				updated_at = NOW()
		`, docID, versionID, processingJobMetadataJSON(map[string]interface{}{
			"source":         "document_upload",
			"requestId":      c.GetString("requestID"),
			"lastTransition": "QUEUED",
		}))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create processing job: %v", err)})
			return
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO audit.events (organization_id, plant_id, actor_user_id, event_type, resource_type, resource_id, correlation_id, metadata_json)
			VALUES ($1, $2, $3, 'DOCUMENT_UPLOADED', 'document', $4, $5, $6)
		`, orgID, plantID, userID, docID.String(), c.GetString("requestID"), auditMetadataJSON(c, map[string]interface{}{
			"fileName":     header.Filename,
			"fileSha256":   fileSHA256,
			"objectKey":    objectKey,
			"accessLevel":  accessPolicy.AccessLevel,
			"sensitivity":  accessPolicy.Sensitivity,
			"allowedRoles": accessPolicy.AllowedRoles,
		}))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to record audit event: %v", err)})
			return
		}

		if err = tx.Commit(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to commit transaction: %v", err)})
			return
		}

		processReq := map[string]interface{}{
			"document_id": docID.String(),
			"file_path":   localFilePath,
			"file_type":   strings.TrimPrefix(ext, "."),
			"metadata": map[string]interface{}{
				"plantId":        plantID,
				"documentType":   documentType,
				"title":          header.Filename,
				"organizationId": orgID,
				"accessLevel":    accessPolicy.AccessLevel,
				"sensitivity":    accessPolicy.Sensitivity,
				"allowedRoles":   accessPolicy.AllowedRoles,
			},
		}

		dispatchAttempts, err := lockProcessingJobForDispatch(ctx, dbPool, docID.String(), versionID.String(), c.GetString("requestID"))
		if err != nil {
			reason := fmt.Sprintf("failed to lock processing job for dispatch: %v", err)
			_ = upsertProcessingJob(ctx, dbPool, docID.String(), versionID.String(), orgID, "FAILED", reason, 0, true)
			c.JSON(http.StatusInternalServerError, gin.H{"error": reason})
			return
		}

		aiResp, err := triggerDocumentProcessing(ctx, aiServiceURL, processReq)
		if err != nil {
			reason := aiFailureReason(err, aiResp)
			log.Printf("Warning: Failed to call Python AI Service: %v", err)
			_ = completeProcessingDispatch(ctx, dbPool, docID.String(), versionID.String(), orgID, "FAILED", reason, 0, dispatchAttempts, map[string]interface{}{
				"aiStatusCode": aiResp.StatusCode,
				"requestId":    c.GetString("requestID"),
			})
			recordAuditEvent(c, dbPool, plantID, "DOCUMENT_PROCESSING_TRIGGER_FAILED", "document", docID.String(), map[string]interface{}{
				"reason":   reason,
				"attempts": dispatchAttempts,
			})
			c.JSON(http.StatusOK, gin.H{
				"documentId":         docID.String(),
				"status":             "FAILED",
				"processingStatus":   "FAILED",
				"processingProgress": 0,
				"processingAttempts": dispatchAttempts,
				"attempts":           dispatchAttempts,
				"retryAvailable":     true,
				"retryUrl":           fmt.Sprintf("/api/documents/%s/retry-processing", docID.String()),
				"accessLevel":        accessPolicy.AccessLevel,
				"sensitivity":        accessPolicy.Sensitivity,
				"allowedRoles":       accessPolicy.AllowedRoles,
				"message":            "Document uploaded, but AI ingestion trigger failed and can be retried",
			})
			return
		}

		if aiResp.StatusCode != http.StatusOK && aiResp.StatusCode != http.StatusAccepted {
			reason := aiFailureReason(nil, aiResp)
			log.Printf("Warning: AI service returned status %d: %s", aiResp.StatusCode, string(aiResp.Body))
			_ = completeProcessingDispatch(ctx, dbPool, docID.String(), versionID.String(), orgID, "FAILED", reason, 0, dispatchAttempts, map[string]interface{}{
				"aiStatusCode": aiResp.StatusCode,
				"requestId":    c.GetString("requestID"),
			})
			recordAuditEvent(c, dbPool, plantID, "DOCUMENT_PROCESSING_TRIGGER_FAILED", "document", docID.String(), map[string]interface{}{
				"reason":       reason,
				"attempts":     dispatchAttempts,
				"aiStatusCode": aiResp.StatusCode,
			})
			c.JSON(http.StatusOK, gin.H{
				"documentId":         docID.String(),
				"status":             "FAILED",
				"processingStatus":   "FAILED",
				"processingProgress": 0,
				"processingAttempts": dispatchAttempts,
				"attempts":           dispatchAttempts,
				"retryAvailable":     true,
				"retryUrl":           fmt.Sprintf("/api/documents/%s/retry-processing", docID.String()),
				"accessLevel":        accessPolicy.AccessLevel,
				"sensitivity":        accessPolicy.Sensitivity,
				"allowedRoles":       accessPolicy.AllowedRoles,
				"message":            "Document uploaded, but AI ingestion was not accepted and can be retried",
			})
			return
		}

		_ = completeProcessingDispatch(ctx, dbPool, docID.String(), versionID.String(), orgID, "QUEUED", "", 0, dispatchAttempts, map[string]interface{}{
			"aiStatusCode": aiResp.StatusCode,
			"requestId":    c.GetString("requestID"),
		})
		recordAuditEvent(c, dbPool, plantID, "DOCUMENT_PROCESSING_QUEUED", "document", docID.String(), map[string]interface{}{
			"aiStatusCode": aiResp.StatusCode,
			"attempts":     dispatchAttempts,
		})
		c.JSON(http.StatusOK, gin.H{
			"documentId":         docID.String(),
			"status":             "UPLOADED",
			"processingStatus":   "QUEUED",
			"processingProgress": 0,
			"processingAttempts": dispatchAttempts,
			"attempts":           dispatchAttempts,
			"retryAvailable":     false,
			"retryUrl":           "",
			"accessLevel":        accessPolicy.AccessLevel,
			"sensitivity":        accessPolicy.Sensitivity,
			"allowedRoles":       accessPolicy.AllowedRoles,
			"message":            "Document uploaded and queued for processing",
		})
	}
}

// HandleDocumentVersionUpload registers a new version for an existing document.
func HandleDocumentVersionUpload(dbPool *pgxpool.Pool, minioClient *minio.Client, bucketName, aiServiceURL, s3Endpoint string) gin.HandlerFunc {
	return func(c *gin.Context) {
		docID := c.Param("id")
		orgID := c.GetString("orgID")
		userID := c.GetString("userID")
		ctx := c.Request.Context()

		file, header, err := c.Request.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "file field is required"})
			return
		}
		defer file.Close()

		var plantID, title, documentType, accessLevel, sensitivity string
		var allowedRoles []string
		var versionCount int
		var latestJobStatus *string
		err = dbPool.QueryRow(ctx, `
			SELECT
				d.plant_id::text,
				d.title,
				COALESCE(d.document_type, ''),
				COALESCE(d.access_level, 'internal'),
				COALESCE(d.sensitivity, 'standard'),
				COALESCE(d.allowed_roles, ARRAY[]::text[]),
				(SELECT COUNT(*) FROM document.document_versions WHERE document_id = d.id),
				j.status
			FROM document.documents d
			LEFT JOIN LATERAL (
				SELECT status
				FROM ingestion.processing_jobs
				WHERE document_id = d.id AND document_version_id = d.current_version_id
				ORDER BY updated_at DESC, created_at DESC
				LIMIT 1
			) j ON TRUE
			WHERE d.id = $1
			  AND d.organization_id = $2
			  AND d.status <> 'ARCHIVED'
		`, docID, orgID).Scan(&plantID, &title, &documentType, &accessLevel, &sensitivity, &allowedRoles, &versionCount, &latestJobStatus)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
			return
		}
		if !auth.RequirePlantAccess(c, dbPool, plantID) {
			return
		}
		if !auth.RequireDocumentAccess(c, accessLevel, allowedRoles) {
			return
		}
		if latestJobStatus != nil && activeProcessingStatus(*latestJobStatus) {
			c.JSON(http.StatusConflict, gin.H{
				"documentId": docID,
				"status":     *latestJobStatus,
				"message":    "cannot upload a new version while processing is active",
			})
			return
		}

		ext := filepath.Ext(header.Filename)
		const maxVersionUploadSize = 50 * 1024 * 1024 // 50 MB
		if header.Size > maxVersionUploadSize {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file size exceeds 50MB limit"})
			return
		}
		if !allowedDocumentExtension(ext) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported file type"})
			return
		}
		if strings.ToLower(ext) == ".pdf" {
			buf := make([]byte, 5)
			if _, err := io.ReadFull(file, buf); err != nil || string(buf) != "%PDF-" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "corrupt PDF file"})
				return
			}
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to reset uploaded file: %v", err)})
				return
			}
		}

		uploadsDir := os.Getenv("UPLOADS_DIR")
		if uploadsDir == "" {
			uploadsDir = "/app/uploads"
		}
		if err := os.MkdirAll(uploadsDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create upload directory: %v", err)})
			return
		}

		localFilePath := filepath.Join(uploadsDir, fmt.Sprintf("%s%s", docID, ext))
		out, err := os.Create(localFilePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create local file: %v", err)})
			return
		}
		hasher := sha256.New()
		if _, err := io.Copy(io.MultiWriter(out, hasher), file); err != nil {
			out.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to save local file: %v", err)})
			return
		}
		out.Close()

		fileSHA256 := hex.EncodeToString(hasher.Sum(nil))
		localStat, err := os.Stat(localFilePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to inspect uploaded file: %v", err)})
			return
		}
		if localStat.Size() == 0 {
			_ = os.Remove(localFilePath)
			c.JSON(http.StatusBadRequest, gin.H{"error": "uploaded file is empty"})
			return
		}

		var duplicateVersion string
		err = dbPool.QueryRow(ctx, `
			SELECT version_label
			FROM document.document_versions
			WHERE document_id = $1
			  AND file_sha256 = $2
			ORDER BY created_at DESC
			LIMIT 1
		`, docID, fileSHA256).Scan(&duplicateVersion)
		if err == nil {
			_ = os.Remove(localFilePath)
			c.JSON(http.StatusOK, gin.H{
				"documentId": docID,
				"status":     "DUPLICATE_VERSION",
				"duplicate":  true,
				"message":    fmt.Sprintf("This file already exists as %s", duplicateVersion),
			})
			return
		}
		if err != pgx.ErrNoRows {
			_ = os.Remove(localFilePath)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to check duplicate version: %v", err)})
			return
		}

		versionLabel := strings.TrimSpace(c.PostForm("versionLabel"))
		if versionLabel == "" {
			versionLabel = strings.TrimSpace(c.PostForm("version"))
		}
		if versionLabel == "" {
			versionLabel = fmt.Sprintf("v%d", versionCount+1)
		}
		if len(versionLabel) > 40 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "version label must be 40 characters or fewer"})
			return
		}

		minioFile, err := os.Open(localFilePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to open file for MinIO upload: %v", err)})
			return
		}
		defer minioFile.Close()
		fileStat, err := minioFile.Stat()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to stat file for MinIO upload: %v", err)})
			return
		}

		objectKey := fmt.Sprintf("uploads/%s/%s/%s", docID, safeFilePart(versionLabel), filepath.Base(header.Filename))
		_, err = minioClient.PutObject(ctx, bucketName, objectKey, minioFile, fileStat.Size(), minio.PutObjectOptions{
			ContentType: header.Header.Get("Content-Type"),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to upload to MinIO: %v", err)})
			return
		}
		fileURL := fmt.Sprintf("%s/%s/%s", strings.TrimRight(s3Endpoint, "/"), bucketName, objectKey)
		versionID := uuid.New()
		nextTitle := strings.TrimSpace(c.PostForm("title"))
		if nextTitle == "" {
			nextTitle = title
		}

		tx, err := dbPool.Begin(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to start database transaction: %v", err)})
			return
		}
		defer tx.Rollback(ctx)

		if err := cleanupDocumentDerivedData(ctx, tx, docID, orgID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to clear previous derived data: %v", err)})
			return
		}

		if _, err = tx.Exec(ctx, `
			INSERT INTO document.document_versions (id, document_id, version_label, file_url, file_type, file_sha256, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		`, versionID, docID, versionLabel, fileURL, strings.TrimPrefix(ext, "."), fileSHA256); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to insert document version: %v", err)})
			return
		}

		if _, err = tx.Exec(ctx, `
			UPDATE document.documents
			SET current_version_id = $1,
			    title = $2,
			    status = 'UPLOADED',
			    updated_at = NOW()
			WHERE id = $3 AND organization_id = $4
		`, versionID, nextTitle, docID, orgID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to activate document version: %v", err)})
			return
		}

		if _, err = tx.Exec(ctx, `
			INSERT INTO document.upload_sessions (document_id, organization_id, plant_id, object_key, status, created_by, created_at, completed_at)
			VALUES ($1, $2, $3, $4, 'COMPLETED', $5, NOW(), NOW())
		`, docID, orgID, plantID, objectKey, userID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to record upload session: %v", err)})
			return
		}

		if _, err = tx.Exec(ctx, `
			INSERT INTO ingestion.processing_jobs (
				document_id, document_version_id, status, attempts, progress,
				next_retry_at, metadata_json, created_at, updated_at
			)
			VALUES ($1, $2, 'QUEUED', 0, 0, NOW(), $3, NOW(), NOW())
			ON CONFLICT (document_id, document_version_id)
			DO UPDATE SET
				status = 'QUEUED',
				error_message = NULL,
				progress = 0,
				next_retry_at = NOW(),
				locked_at = NULL,
				locked_by = NULL,
				metadata_json = ingestion.processing_jobs.metadata_json || EXCLUDED.metadata_json,
				updated_at = NOW()
		`, docID, versionID, processingJobMetadataJSON(map[string]interface{}{
			"source":         "document_version_upload",
			"requestId":      c.GetString("requestID"),
			"versionLabel":   versionLabel,
			"lastTransition": "QUEUED",
		})); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create processing job: %v", err)})
			return
		}

		if _, err = tx.Exec(ctx, `
			INSERT INTO audit.events (organization_id, plant_id, actor_user_id, event_type, resource_type, resource_id, correlation_id, metadata_json)
			VALUES ($1, $2, $3, 'DOCUMENT_VERSION_UPLOADED', 'document', $4, $5, $6)
		`, orgID, plantID, userID, docID, c.GetString("requestID"), auditMetadataJSON(c, map[string]interface{}{
			"versionId":    versionID.String(),
			"versionLabel": versionLabel,
			"fileName":     header.Filename,
			"fileSHA256":   fileSHA256,
			"objectKey":    objectKey,
			"accessLevel":  accessLevel,
			"sensitivity":  sensitivity,
		})); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to record audit event: %v", err)})
			return
		}

		if err = tx.Commit(ctx); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to commit document version: %v", err)})
			return
		}

		processReq := map[string]interface{}{
			"document_id": docID,
			"file_path":   localFilePath,
			"file_type":   strings.TrimPrefix(ext, "."),
			"metadata": map[string]interface{}{
				"plantId":        plantID,
				"documentType":   documentType,
				"title":          nextTitle,
				"organizationId": orgID,
				"accessLevel":    accessLevel,
				"sensitivity":    sensitivity,
				"allowedRoles":   allowedRoles,
				"versionLabel":   versionLabel,
			},
		}
		dispatchAttempts, err := lockProcessingJobForDispatch(ctx, dbPool, docID, versionID.String(), c.GetString("requestID"))
		if err != nil {
			reason := fmt.Sprintf("failed to lock processing job for dispatch: %v", err)
			_ = upsertProcessingJob(ctx, dbPool, docID, versionID.String(), orgID, "FAILED", reason, 0, true)
			c.JSON(http.StatusInternalServerError, gin.H{"error": reason})
			return
		}

		aiResp, err := triggerDocumentProcessing(ctx, aiServiceURL, processReq)
		if err != nil || (aiResp.StatusCode != http.StatusOK && aiResp.StatusCode != http.StatusAccepted) {
			reason := aiFailureReason(err, aiResp)
			_ = completeProcessingDispatch(ctx, dbPool, docID, versionID.String(), orgID, "FAILED", reason, 0, dispatchAttempts, map[string]interface{}{
				"aiStatusCode": aiResp.StatusCode,
				"requestId":    c.GetString("requestID"),
				"versionLabel": versionLabel,
			})
			recordAuditEvent(c, dbPool, plantID, "DOCUMENT_VERSION_PROCESSING_TRIGGER_FAILED", "document", docID, map[string]interface{}{
				"reason":       reason,
				"versionId":    versionID.String(),
				"versionLabel": versionLabel,
				"attempts":     dispatchAttempts,
			})
			c.JSON(http.StatusOK, gin.H{
				"documentId":         docID,
				"versionId":          versionID.String(),
				"versionLabel":       versionLabel,
				"status":             "FAILED",
				"processingStatus":   "FAILED",
				"processingProgress": 0,
				"processingAttempts": dispatchAttempts,
				"attempts":           dispatchAttempts,
				"retryAvailable":     true,
				"retryUrl":           fmt.Sprintf("/api/documents/%s/retry-processing", docID),
				"message":            "Document version uploaded, but AI ingestion trigger failed and can be retried",
			})
			return
		}

		_ = completeProcessingDispatch(ctx, dbPool, docID, versionID.String(), orgID, "QUEUED", "", 0, dispatchAttempts, map[string]interface{}{
			"aiStatusCode": aiResp.StatusCode,
			"requestId":    c.GetString("requestID"),
			"versionLabel": versionLabel,
		})
		recordAuditEvent(c, dbPool, plantID, "DOCUMENT_VERSION_PROCESSING_QUEUED", "document", docID, map[string]interface{}{
			"aiStatusCode": aiResp.StatusCode,
			"versionId":    versionID.String(),
			"versionLabel": versionLabel,
			"attempts":     dispatchAttempts,
		})
		c.JSON(http.StatusAccepted, gin.H{
			"documentId":         docID,
			"versionId":          versionID.String(),
			"versionLabel":       versionLabel,
			"status":             "UPLOADED",
			"processingStatus":   "QUEUED",
			"processingProgress": 0,
			"processingAttempts": dispatchAttempts,
			"attempts":           dispatchAttempts,
			"retryAvailable":     false,
			"retryUrl":           "",
			"message":            "Document version uploaded and queued for processing",
		})
	}
}

// HandleRetryDocumentProcessing retries the AI ingestion pipeline on a failed document using cached local file.
func HandleRetryDocumentProcessing(dbPool *pgxpool.Pool, aiServiceURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		docID := c.Param("id")
		orgID := c.GetString("orgID")
		ctx := c.Request.Context()

		var plantID, title, documentType, accessLevel, sensitivity, versionID, fileType string
		var allowedRoles []string
		var latestJobStatus *string
		err := dbPool.QueryRow(ctx, `
			SELECT
				d.plant_id::text,
				d.title,
				COALESCE(d.document_type, ''),
				COALESCE(d.access_level, 'internal'),
				COALESCE(d.sensitivity, 'standard'),
				COALESCE(d.allowed_roles, ARRAY[]::text[]),
				v.id::text,
				v.file_type,
				j.status
			FROM document.documents d
			JOIN document.document_versions v ON d.current_version_id = v.id
			LEFT JOIN LATERAL (
				SELECT status
				FROM ingestion.processing_jobs
				WHERE document_id = d.id AND document_version_id = v.id
				ORDER BY updated_at DESC, created_at DESC
				LIMIT 1
			) j ON TRUE
			WHERE d.id = $1
			  AND d.organization_id = $2
			  AND d.status <> 'ARCHIVED'
		`, docID, orgID).Scan(&plantID, &title, &documentType, &accessLevel, &sensitivity, &allowedRoles, &versionID, &fileType, &latestJobStatus)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
			return
		}
		if !auth.RequirePlantAccess(c, dbPool, plantID) {
			return
		}
		if !auth.RequireDocumentAccess(c, accessLevel, allowedRoles) {
			return
		}
		if latestJobStatus != nil && activeProcessingStatus(*latestJobStatus) {
			c.JSON(http.StatusConflict, gin.H{
				"documentId":       docID,
				"status":           *latestJobStatus,
				"processingStatus": *latestJobStatus,
				"retryAvailable":   false,
				"message":          "document processing is already active",
			})
			return
		}

		localFilePath := uploadedFilePath(docID, fileType)
		if _, err := os.Stat(localFilePath); err != nil {
			recordAuditEvent(c, dbPool, plantID, "DOCUMENT_PROCESSING_RETRY_MISSING_FILE", "document", docID, map[string]interface{}{
				"filePath": localFilePath,
				"reason":   err.Error(),
			})
			c.JSON(http.StatusConflict, gin.H{
				"documentId":       docID,
				"status":           "FAILED",
				"processingStatus": "FAILED",
				"retryAvailable":   false,
				"message":          "cannot retry processing because the shared local upload file is missing",
			})
			return
		}

		if err := upsertProcessingJob(ctx, dbPool, docID, versionID, orgID, "QUEUED", "", 0, false); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to queue processing retry: %v", err)})
			return
		}
		dispatchAttempts, err := lockProcessingJobForDispatch(ctx, dbPool, docID, versionID, c.GetString("requestID"))
		if err != nil {
			reason := fmt.Sprintf("failed to lock processing job for retry dispatch: %v", err)
			_ = upsertProcessingJob(ctx, dbPool, docID, versionID, orgID, "FAILED", reason, 0, true)
			c.JSON(http.StatusInternalServerError, gin.H{"error": reason})
			return
		}

		processReq := map[string]interface{}{
			"document_id": docID,
			"file_path":   localFilePath,
			"file_type":   fileType,
			"metadata": map[string]interface{}{
				"plantId":        plantID,
				"documentType":   documentType,
				"title":          title,
				"organizationId": orgID,
				"accessLevel":    accessLevel,
				"sensitivity":    sensitivity,
				"allowedRoles":   allowedRoles,
				"retry":          true,
			},
		}

		aiResp, err := triggerDocumentProcessing(ctx, aiServiceURL, processReq)
		if err != nil || (aiResp.StatusCode != http.StatusOK && aiResp.StatusCode != http.StatusAccepted) {
			reason := aiFailureReason(err, aiResp)
			_ = completeProcessingDispatch(ctx, dbPool, docID, versionID, orgID, "FAILED", reason, 0, dispatchAttempts, map[string]interface{}{
				"aiStatusCode": aiResp.StatusCode,
				"requestId":    c.GetString("requestID"),
				"manualRetry":  true,
			})
			recordAuditEvent(c, dbPool, plantID, "DOCUMENT_PROCESSING_RETRY_FAILED", "document", docID, map[string]interface{}{
				"reason":   reason,
				"attempts": dispatchAttempts,
			})
			c.JSON(http.StatusOK, gin.H{
				"documentId":         docID,
				"status":             "FAILED",
				"processingStatus":   "FAILED",
				"processingProgress": 0,
				"processingAttempts": dispatchAttempts,
				"attempts":           dispatchAttempts,
				"retryAvailable":     true,
				"retryUrl":           fmt.Sprintf("/api/documents/%s/retry-processing", docID),
				"message":            "processing retry could not be accepted by the AI service",
			})
			return
		}

		_ = completeProcessingDispatch(ctx, dbPool, docID, versionID, orgID, "QUEUED", "", 0, dispatchAttempts, map[string]interface{}{
			"aiStatusCode": aiResp.StatusCode,
			"requestId":    c.GetString("requestID"),
			"manualRetry":  true,
		})
		recordAuditEvent(c, dbPool, plantID, "DOCUMENT_PROCESSING_RETRIED", "document", docID, map[string]interface{}{
			"aiStatusCode": aiResp.StatusCode,
			"attempts":     dispatchAttempts,
		})
		c.JSON(http.StatusAccepted, gin.H{
			"documentId":         docID,
			"status":             "QUEUED",
			"processingStatus":   "QUEUED",
			"processingProgress": 0,
			"processingAttempts": dispatchAttempts,
			"attempts":           dispatchAttempts,
			"retryAvailable":     false,
			"retryUrl":           "",
			"message":            "Document processing retry queued",
		})
	}
}

func storageBaseURL(useSSL bool, endpoint string) string {
	scheme := "http"
	if useSSL {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, strings.TrimRight(endpoint, "/"))
}

func cleanupDocumentDerivedData(ctx context.Context, tx pgx.Tx, docID, orgID string) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM graph.relationships r
		USING graph.entities e
		WHERE (r.source_entity_id = e.id OR r.target_entity_id = e.id)
		  AND e.document_id = $1
		  AND r.organization_id = $2
	`, docID, orgID); err != nil {
		return err
	}

	statements := []string{
		`DELETE FROM graph.relationships WHERE evidence_document_id = $1 AND organization_id = $2`,
		`DELETE FROM graph.entities WHERE document_id = $1 AND organization_id = $2`,
		`DELETE FROM rag.citations WHERE document_id = $1`,
		`DELETE FROM ingestion.document_chunks WHERE document_id = $1`,
		`DELETE FROM ingestion.document_pages WHERE document_id = $1`,
	}
	for _, stmt := range statements {
		if strings.Contains(stmt, "$2") {
			if _, err := tx.Exec(ctx, stmt, docID, orgID); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(ctx, stmt, docID); err != nil {
			return err
		}
	}
	return nil
}

func allowedDocumentExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".pdf", ".docx", ".doc", ".xlsx", ".xls", ".csv", ".png", ".jpg", ".jpeg", ".txt", ".md":
		return true
	default:
		return false
	}
}

func terminalProcessingStatus(status string) bool {
	switch strings.ToUpper(status) {
	case "COMPLETED", "FAILED", "PARTIAL_SUCCESS", "ARCHIVED":
		return true
	default:
		return false
	}
}

func activeProcessingStatus(status string) bool {
	switch strings.ToUpper(status) {
	case "QUEUED", "DISPATCHING", "EXTRACTING_TEXT", "OCR_RUNNING", "CONVERTING_TO_MARKDOWN", "DETECTING_TABLES", "CHUNKING", "EXTRACTING_ENTITIES", "GENERATING_EMBEDDINGS", "STORING_RESULTS":
		return true
	default:
		return false
	}
}

func documentStatusForProcessingJob(status string) string {
	if strings.EqualFold(status, "QUEUED") {
		return "UPLOADED"
	}
	return status
}

func processingRetryDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := processingRetryBaseDelay
	for i := 1; i < attempts; i++ {
		delay *= 2
		if delay >= processingRetryMaxDelay {
			return processingRetryMaxDelay
		}
	}
	return delay
}

func processingNextRetryAt(status string, attempts int, now time.Time) *time.Time {
	switch strings.ToUpper(status) {
	case "QUEUED":
		retryAt := now
		return &retryAt
	case "FAILED":
		retryAt := now.Add(processingRetryDelay(attempts))
		return &retryAt
	default:
		return nil
	}
}

func processingJobMetadataJSON(metadata map[string]interface{}) []byte {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return []byte("{}")
	}
	return payload
}

func upsertProcessingJob(ctx context.Context, dbPool *pgxpool.Pool, documentID, documentVersionID, orgID, status, errorMessage string, progress float64, incrementAttempts bool) error {
	attemptIncrement := 0
	if incrementAttempts {
		attemptIncrement = 1
	}
	completed := terminalProcessingStatus(status)
	documentStatus := documentStatusForProcessingJob(status)
	now := time.Now().UTC()
	nextRetryAt := processingNextRetryAt(status, attemptIncrement, now)
	metadataJSON := processingJobMetadataJSON(map[string]interface{}{
		"lastTransition": status,
	})

	_, err := dbPool.Exec(ctx, `
		UPDATE document.documents
		SET status = $1, updated_at = NOW()
		WHERE id = $2 AND organization_id = $3
	`, documentStatus, documentID, orgID)
	if err != nil {
		return err
	}

	_, err = dbPool.Exec(ctx, `
		INSERT INTO ingestion.processing_jobs (
			document_id, document_version_id, status, error_message, attempts, progress,
			last_attempted_at, next_retry_at, locked_at, locked_by, metadata_json,
			started_at, completed_at, created_at, updated_at
		)
		VALUES (
			$1, $2, $3, NULLIF($4, ''), $5, $6,
			CASE WHEN $5 > 0 THEN NOW() ELSE NULL END, $8, NULL, NULL, $9,
			NOW(), CASE WHEN $7 THEN NOW() ELSE NULL END, NOW(), NOW()
		)
		ON CONFLICT (document_id, document_version_id)
		DO UPDATE SET
			status = EXCLUDED.status,
			error_message = EXCLUDED.error_message,
			progress = EXCLUDED.progress,
			attempts = ingestion.processing_jobs.attempts + $5,
			last_attempted_at = CASE WHEN $5 > 0 THEN NOW() ELSE ingestion.processing_jobs.last_attempted_at END,
			next_retry_at = $8,
			locked_at = NULL,
			locked_by = NULL,
			metadata_json = ingestion.processing_jobs.metadata_json || EXCLUDED.metadata_json,
			started_at = CASE WHEN $5 > 0 THEN NOW() ELSE COALESCE(ingestion.processing_jobs.started_at, NOW()) END,
			completed_at = CASE WHEN $7 THEN NOW() ELSE NULL END,
			updated_at = NOW()
	`, documentID, documentVersionID, status, errorMessage, attemptIncrement, progress, completed, nextRetryAt, metadataJSON)
	return err
}

func lockProcessingJobForDispatch(ctx context.Context, dbPool *pgxpool.Pool, documentID, documentVersionID, lockedBy string) (int, error) {
	var attempts int
	err := dbPool.QueryRow(ctx, `
		UPDATE ingestion.processing_jobs
		SET status = 'DISPATCHING',
		    attempts = attempts + 1,
		    last_attempted_at = NOW(),
		    next_retry_at = NULL,
		    locked_at = NOW(),
		    locked_by = NULLIF($3, ''),
		    error_message = NULL,
		    metadata_json = metadata_json || $4,
		    updated_at = NOW()
		WHERE document_id = $1 AND document_version_id = $2
		RETURNING attempts
	`, documentID, documentVersionID, lockedBy, processingJobMetadataJSON(map[string]interface{}{
		"lastTransition": "DISPATCHING",
		"lockedBy":       lockedBy,
	})).Scan(&attempts)
	return attempts, err
}

func completeProcessingDispatch(ctx context.Context, dbPool *pgxpool.Pool, documentID, documentVersionID, orgID, status, errorMessage string, progress float64, attempts int, metadata map[string]interface{}) error {
	completed := terminalProcessingStatus(status)
	documentStatus := documentStatusForProcessingJob(status)
	nextRetryAt := processingNextRetryAt(status, attempts, time.Now().UTC())
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata["lastTransition"] = status

	_, err := dbPool.Exec(ctx, `
		UPDATE document.documents
		SET status = $1, updated_at = NOW()
		WHERE id = $2 AND organization_id = $3
	`, documentStatus, documentID, orgID)
	if err != nil {
		return err
	}

	_, err = dbPool.Exec(ctx, `
		UPDATE ingestion.processing_jobs
		SET status = $3,
		    error_message = NULLIF($4, ''),
		    progress = $5,
		    next_retry_at = $6,
		    locked_at = NULL,
		    locked_by = NULL,
		    metadata_json = metadata_json || $7,
		    completed_at = CASE WHEN $8 THEN NOW() ELSE NULL END,
		    updated_at = NOW()
		WHERE document_id = $1 AND document_version_id = $2
	`, documentID, documentVersionID, status, errorMessage, progress, nextRetryAt, processingJobMetadataJSON(metadata), completed)
	return err
}

func uploadedFilePath(documentID, fileType string) string {
	uploadsDir := os.Getenv("UPLOADS_DIR")
	if uploadsDir == "" {
		uploadsDir = "/app/uploads"
	}
	return filepath.Join(uploadsDir, fmt.Sprintf("%s.%s", documentID, strings.TrimPrefix(strings.ToLower(fileType), ".")))
}

func triggerDocumentProcessing(ctx context.Context, aiServiceURL string, processReq map[string]interface{}) (aiServiceResponse, error) {
	payloadBytes, err := json.Marshal(processReq)
	if err != nil {
		return aiServiceResponse{}, err
	}
	return callAIService(ctx, http.MethodPost, aiServiceURL, "/process-document", payloadBytes)
}

func safeDownloadFileName(title, fileType string) string {
	name := strings.TrimSpace(filepath.Base(title))
	if name == "." || name == "/" || name == "" {
		name = "document"
	}
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	if filepath.Ext(name) == "" && fileType != "" {
		name = fmt.Sprintf("%s.%s", name, strings.TrimPrefix(strings.ToLower(fileType), "."))
	}
	return name
}

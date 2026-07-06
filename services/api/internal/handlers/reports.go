package handlers

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"plantbrain-api/internal/auth"
	"plantbrain-api/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type reportTable struct {
	Title   string
	Headers []string
	Rows    [][]string
}

// HandleListReportJobs retrieves report jobs requested by users.
func HandleListReportJobs(dbPool *pgxpool.Pool) gin.HandlerFunc {
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
			SELECT id::text, report_type, status, output_file_url, error_message, created_at, completed_at
			FROM report.jobs
			WHERE organization_id = $1
			  AND ($2::uuid IS NULL OR plant_id = $2::uuid)
			ORDER BY created_at DESC
			LIMIT 25
		`, orgID, nullableUUID(plantID))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to list report jobs: %v", err)})
			return
		}
		defer rows.Close()

		jobs := []gin.H{}
		for rows.Next() {
			var id, reportType, status string
			var outputFileURL, errorMessage *string
			var createdAt time.Time
			var completedAt *time.Time
			if err := rows.Scan(&id, &reportType, &status, &outputFileURL, &errorMessage, &createdAt, &completedAt); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to scan report job: %v", err)})
				return
			}

			job := gin.H{
				"id":         id,
				"reportType": reportType,
				"status":     status,
				"createdAt":  createdAt,
			}
			if completedAt != nil {
				job["completedAt"] = completedAt
			}
			if outputFileURL != nil {
				job["downloadUrl"] = fmt.Sprintf("/api/reports/%s/download", id)
			}
			if errorMessage != nil {
				job["errorMessage"] = *errorMessage
			}
			jobs = append(jobs, job)
		}

		c.JSON(http.StatusOK, jobs)
	}
}

// HandleCreateReportJob triggers a new report compilation.
func HandleCreateReportJob(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req models.ReportJobReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		orgID := c.GetString("orgID")
		userID := c.GetString("userID")

		if req.PlantID == "" {
			req.PlantID = c.GetString("plantID")
		}
		if req.PlantID != "" && !auth.RequirePlantAccess(c, dbPool, req.PlantID) {
			return
		}

		var plantIDVal interface{}
		if req.PlantID != "" {
			plantIDVal = req.PlantID
		}

		format, err := normalizeReportFormat(req)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		paramsJSON, err := json.Marshal(req.Parameters)
		if err != nil {
			paramsJSON = []byte("{}")
		}

		ctx := c.Request.Context()
		var jobID string
		var createdAt time.Time
		err = dbPool.QueryRow(ctx, `
			INSERT INTO report.jobs (organization_id, plant_id, requested_by, report_type, status, parameters_json, created_at)
			VALUES ($1, $2, $3, $4, 'RUNNING', $5, NOW())
			RETURNING id::text, created_at
		`, orgID, plantIDVal, userID, req.ReportType, paramsJSON).Scan(&jobID, &createdAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create report job: %v", err)})
			return
		}

		table, err := generateReportTable(ctx, dbPool, orgID, req.PlantID, req.ReportType)
		if err != nil {
			_, _ = dbPool.Exec(ctx, `
				UPDATE report.jobs
				SET status = 'FAILED', error_message = $1, completed_at = NOW()
				WHERE id = $2 AND organization_id = $3
			`, err.Error(), jobID, orgID)
			c.JSON(http.StatusBadRequest, gin.H{"id": jobID, "status": "FAILED", "error": err.Error()})
			return
		}
		reportBytes, err := renderReport(format, table)
		if err != nil {
			_, _ = dbPool.Exec(ctx, `
				UPDATE report.jobs
				SET status = 'FAILED', error_message = $1, completed_at = NOW()
				WHERE id = $2 AND organization_id = $3
			`, err.Error(), jobID, orgID)
			c.JSON(http.StatusBadRequest, gin.H{"id": jobID, "status": "FAILED", "error": err.Error()})
			return
		}

		reportsDir := reportOutputDir()
		if err := os.MkdirAll(reportsDir, 0755); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create reports directory: %v", err)})
			return
		}

		fileName := fmt.Sprintf("%s_%s.%s", jobID, safeFilePart(req.ReportType), reportExtension(format))
		filePath := filepath.Join(reportsDir, fileName)
		if err := os.WriteFile(filePath, reportBytes, 0644); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to write report: %v", err)})
			return
		}

		outputFileURL := fmt.Sprintf("reports/%s", fileName)
		_, err = dbPool.Exec(ctx, `
			UPDATE report.jobs
			SET status = 'COMPLETED', output_file_url = $1, completed_at = NOW()
			WHERE id = $2 AND organization_id = $3
		`, outputFileURL, jobID, orgID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update report job: %v", err)})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"id":          jobID,
			"status":      "COMPLETED",
			"createdAt":   createdAt,
			"format":      format,
			"downloadUrl": fmt.Sprintf("/api/reports/%s/download", jobID),
			"message":     "Report generated successfully",
		})
	}
}

// HandleDownloadReport serves the generated report binary.
func HandleDownloadReport(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("orgID")
		reportID := c.Param("id")

		var outputFileURL string
		var reportType string
		var plantID *string
		err := dbPool.QueryRow(c.Request.Context(), `
			SELECT output_file_url, report_type, plant_id::text
			FROM report.jobs
			WHERE id = $1 AND organization_id = $2 AND status = 'COMPLETED'
		`, reportID, orgID).Scan(&outputFileURL, &reportType, &plantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "completed report not found"})
			return
		}
		if plantID != nil && !auth.RequirePlantAccess(c, dbPool, *plantID) {
			return
		}

		fileName := strings.TrimPrefix(outputFileURL, "reports/")
		filePath := filepath.Join(reportOutputDir(), filepath.Base(fileName))
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(fileName)), ".")
		if ext == "" {
			ext = "csv"
		}
		c.Header("Content-Type", reportContentType(ext))
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fmt.Sprintf("%s_%s.%s", safeFilePart(reportType), shortID(reportID), ext)))
		c.File(filePath)
	}
}

func generateReportTable(ctx context.Context, dbPool *pgxpool.Pool, orgID, plantID, reportType string) (reportTable, error) {
	table := reportTable{
		Title: humanizeIdentifier(reportType) + " Report",
	}
	switch strings.ToLower(reportType) {
	case "asset_summary":
		table.Title = "Asset Summary Report"
		table.Headers = []string{"asset_tag", "asset_name", "asset_type", "location", "criticality", "risk_score"}
		rows, err := dbPool.Query(ctx, `
			SELECT asset_tag, COALESCE(asset_name, ''), COALESCE(asset_type, ''), COALESCE(location, ''), COALESCE(criticality, ''), COALESCE(risk_score, 0)
			FROM asset.assets
			WHERE organization_id = $1 AND ($2::uuid IS NULL OR plant_id = $2::uuid)
			ORDER BY asset_tag
		`, orgID, nullableUUID(plantID))
		if err != nil {
			return table, err
		}
		defer rows.Close()
		for rows.Next() {
			var tag, name, assetType, location, criticality string
			var riskScore float64
			if err := rows.Scan(&tag, &name, &assetType, &location, &criticality, &riskScore); err != nil {
				return table, err
			}
			table.Rows = append(table.Rows, []string{tag, name, assetType, location, criticality, fmt.Sprintf("%.1f", riskScore)})
		}

	case "compliance_gap":
		table.Title = "Compliance Gap Report"
		table.Headers = []string{"asset_tag", "gap_type", "severity", "status", "description", "created_at"}
		rows, err := dbPool.Query(ctx, `
			SELECT COALESCE(a.asset_tag, ''), g.gap_type, COALESCE(g.severity, ''), g.status, g.description, g.created_at
			FROM compliance.gaps g
			LEFT JOIN asset.assets a ON g.asset_id = a.id
			WHERE g.organization_id = $1 AND ($2::uuid IS NULL OR g.plant_id = $2::uuid)
			ORDER BY g.created_at DESC
		`, orgID, nullableUUID(plantID))
		if err != nil {
			return table, err
		}
		defer rows.Close()
		for rows.Next() {
			var tag, gapType, severity, status, description string
			var createdAt time.Time
			if err := rows.Scan(&tag, &gapType, &severity, &status, &description, &createdAt); err != nil {
				return table, err
			}
			table.Rows = append(table.Rows, []string{tag, gapType, severity, status, description, createdAt.Format(time.RFC3339)})
		}

	case "document_inventory":
		table.Title = "Document Inventory Report"
		table.Headers = []string{"title", "document_type", "file_type", "status", "ocr_confidence", "classification_confidence", "created_at"}
		rows, err := dbPool.Query(ctx, `
			SELECT d.title, COALESCE(d.document_type, ''), COALESCE(v.file_type, ''), d.status,
			       COALESCE(v.ocr_confidence, 0), COALESCE(v.classification_confidence, 0), d.created_at
			FROM document.documents d
			LEFT JOIN document.document_versions v ON d.current_version_id = v.id
			WHERE d.organization_id = $1 AND d.status <> 'ARCHIVED' AND ($2::uuid IS NULL OR d.plant_id = $2::uuid)
			ORDER BY d.created_at DESC
		`, orgID, nullableUUID(plantID))
		if err != nil {
			return table, err
		}
		defer rows.Close()
		for rows.Next() {
			var title, documentType, fileType, status string
			var ocrConfidence, classificationConfidence float64
			var createdAt time.Time
			if err := rows.Scan(&title, &documentType, &fileType, &status, &ocrConfidence, &classificationConfidence, &createdAt); err != nil {
				return table, err
			}
			table.Rows = append(table.Rows, []string{title, documentType, fileType, status, fmt.Sprintf("%.2f", ocrConfidence), fmt.Sprintf("%.2f", classificationConfidence), createdAt.Format(time.RFC3339)})
		}

	case "rca_report":
		table.Title = "RCA Report"
		table.Headers = []string{"asset_tag", "failure_summary", "probable_causes", "recommendations", "confidence", "created_at"}
		rows, err := dbPool.Query(ctx, `
			SELECT COALESCE(a.asset_tag, ''), r.failure_summary, r.probable_causes::text, r.recommendations::text, COALESCE(r.confidence, 0), r.created_at
			FROM rca.reports r
			LEFT JOIN asset.assets a ON r.asset_id = a.id
			WHERE r.organization_id = $1 AND ($2::uuid IS NULL OR r.plant_id = $2::uuid OR a.plant_id = $2::uuid)
			ORDER BY r.created_at DESC
		`, orgID, nullableUUID(plantID))
		if err != nil {
			return table, err
		}
		defer rows.Close()
		for rows.Next() {
			var tag, summary, causes, recommendations string
			var confidence float64
			var createdAt time.Time
			if err := rows.Scan(&tag, &summary, &causes, &recommendations, &confidence, &createdAt); err != nil {
				return table, err
			}
			table.Rows = append(table.Rows, []string{tag, summary, causes, recommendations, fmt.Sprintf("%.2f", confidence), createdAt.Format(time.RFC3339)})
		}

	case "query_answers":
		table.Title = "Query Answer Report"
		table.Headers = []string{"query", "answer", "confidence", "created_at"}
		rows, err := dbPool.Query(ctx, `
			SELECT query_text, COALESCE(answer_text, ''), COALESCE(confidence, 0), created_at
			FROM rag.queries
			WHERE organization_id = $1 AND ($2::uuid IS NULL OR plant_id = $2::uuid)
			ORDER BY created_at DESC
		`, orgID, nullableUUID(plantID))
		if err != nil {
			return table, err
		}
		defer rows.Close()
		for rows.Next() {
			var query, answer string
			var confidence float64
			var createdAt time.Time
			if err := rows.Scan(&query, &answer, &confidence, &createdAt); err != nil {
				return table, err
			}
			table.Rows = append(table.Rows, []string{query, answer, fmt.Sprintf("%.2f", confidence), createdAt.Format(time.RFC3339)})
		}

	default:
		return table, fmt.Errorf("unsupported report type %q", reportType)
	}

	return table, nil
}

func normalizeReportFormat(req models.ReportJobReq) (string, error) {
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" && req.Parameters != nil {
		if value, ok := req.Parameters["format"].(string); ok {
			format = strings.ToLower(strings.TrimSpace(value))
		}
	}
	if format == "" {
		format = "csv"
	}
	switch format {
	case "csv", "pdf", "docx":
		return format, nil
	default:
		return "", fmt.Errorf("unsupported report format %q", format)
	}
}

func renderReport(format string, table reportTable) ([]byte, error) {
	switch format {
	case "csv":
		return renderCSVReport(table)
	case "pdf":
		return renderPDFReport(table), nil
	case "docx":
		return renderDOCXReport(table)
	default:
		return nil, fmt.Errorf("unsupported report format %q", format)
	}
}

func renderCSVReport(table reportTable) ([]byte, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write(table.Headers); err != nil {
		return nil, err
	}
	for _, row := range table.Rows {
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func renderPDFReport(table reportTable) []byte {
	lines := reportTextLines(table, 96)
	if len(lines) == 0 {
		lines = []string{table.Title}
	}

	const linesPerPage = 46
	pageCount := (len(lines) + linesPerPage - 1) / linesPerPage
	if pageCount == 0 {
		pageCount = 1
	}

	var objects []string
	objects = append(objects, "<< /Type /Catalog /Pages 2 0 R >>")

	var kids []string
	for i := 0; i < pageCount; i++ {
		pageObjID := 4 + i*2
		kids = append(kids, fmt.Sprintf("%d 0 R", pageObjID))
	}
	objects = append(objects, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), pageCount))
	objects = append(objects, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

	for page := 0; page < pageCount; page++ {
		pageObjID := 4 + page*2
		contentObjID := pageObjID + 1
		start := page * linesPerPage
		end := start + linesPerPage
		if end > len(lines) {
			end = len(lines)
		}

		var stream bytes.Buffer
		stream.WriteString("BT\n/F1 10 Tf\n50 760 Td\n14 TL\n")
		for _, line := range lines[start:end] {
			stream.WriteString("(")
			stream.WriteString(escapePDFText(line))
			stream.WriteString(") Tj\nT*\n")
		}
		stream.WriteString("ET\n")

		objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", contentObjID))
		objects = append(objects, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", stream.Len(), stream.String()))
	}

	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, object := range objects {
		offsets = append(offsets, pdf.Len())
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xrefOffset := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for i := 1; i < len(offsets); i++ {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset)
	return pdf.Bytes()
}

func renderDOCXReport(table reportTable) ([]byte, error) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)

	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`,
		"word/_rels/document.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`,
		"word/document.xml": buildDOCXDocumentXML(table),
	}

	for name, content := range files {
		file, err := archive.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := file.Write([]byte(content)); err != nil {
			return nil, err
		}
	}

	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func buildDOCXDocumentXML(table reportTable) string {
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	body.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	body.WriteString(docxParagraph(table.Title))
	body.WriteString(docxParagraph("Generated: " + time.Now().Format(time.RFC3339)))
	body.WriteString(`<w:tbl>`)
	body.WriteString(`<w:tr>`)
	for _, header := range table.Headers {
		body.WriteString(docxCell(header))
	}
	body.WriteString(`</w:tr>`)
	for _, row := range table.Rows {
		body.WriteString(`<w:tr>`)
		for _, value := range row {
			body.WriteString(docxCell(value))
		}
		body.WriteString(`</w:tr>`)
	}
	body.WriteString(`</w:tbl>`)
	body.WriteString(`<w:sectPr><w:pgSz w:w="12240" w:h="15840"/><w:pgMar w:top="720" w:right="720" w:bottom="720" w:left="720"/></w:sectPr>`)
	body.WriteString(`</w:body></w:document>`)
	return body.String()
}

func docxParagraph(text string) string {
	return `<w:p><w:r><w:t>` + xmlEscape(text) + `</w:t></w:r></w:p>`
}

func docxCell(text string) string {
	return `<w:tc><w:p><w:r><w:t>` + xmlEscape(text) + `</w:t></w:r></w:p></w:tc>`
}

func reportTextLines(table reportTable, width int) []string {
	lines := []string{
		table.Title,
		"Generated: " + time.Now().Format(time.RFC3339),
		"",
		strings.Join(table.Headers, " | "),
		strings.Repeat("-", width),
	}
	for _, row := range table.Rows {
		for _, line := range wrapText(strings.Join(row, " | "), width) {
			lines = append(lines, line)
		}
		lines = append(lines, "")
	}
	if len(table.Rows) == 0 {
		lines = append(lines, "No records found for this report scope.")
	}
	return lines
}

func wrapText(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	var line string
	for _, word := range words {
		if line == "" {
			line = word
			continue
		}
		if len(line)+1+len(word) > width {
			lines = append(lines, line)
			line = word
			continue
		}
		line += " " + word
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func escapePDFText(text string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`, "\r", " ", "\n", " ")
	return replacer.Replace(text)
}

func xmlEscape(text string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return replacer.Replace(text)
}

func humanizeIdentifier(value string) string {
	parts := strings.Fields(strings.ReplaceAll(value, "_", " "))
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func reportOutputDir() string {
	base := os.Getenv("UPLOADS_DIR")
	if base == "" {
		base = "/app/uploads"
	}
	return filepath.Join(base, "reports")
}

func safeFilePart(value string) string {
	cleaned := strings.ToLower(value)
	cleaned = strings.ReplaceAll(cleaned, " ", "_")
	cleaned = strings.ReplaceAll(cleaned, "/", "_")
	cleaned = strings.ReplaceAll(cleaned, "\\", "_")
	if cleaned == "" {
		return "report"
	}
	return cleaned
}

func reportExtension(format string) string {
	switch strings.ToLower(format) {
	case "pdf":
		return "pdf"
	case "docx":
		return "docx"
	default:
		return "csv"
	}
}

func reportContentType(format string) string {
	switch strings.ToLower(format) {
	case "pdf":
		return "application/pdf"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return "text/csv; charset=utf-8"
	}
}

func shortID(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}

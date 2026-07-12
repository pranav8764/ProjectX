package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Asset represents the asset table structure
type Asset struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	PlantID        string    `json:"plantId"`
	AssetTag       string    `json:"assetTag"`
	AssetName      string    `json:"assetName"`
	AssetType      string    `json:"assetType"`
	Location       string    `json:"location"`
	Criticality    string    `json:"criticality"`
	RiskScore      float64   `json:"riskScore"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// Failure represents a simplified RCA report associated with an asset
type Failure struct {
	ID              string    `json:"id"`
	FailureSummary  string    `json:"failureSummary"`
	Timeline        []string  `json:"timeline,omitempty"`
	ProbableCauses  []string  `json:"probableCauses"`
	Recommendations []string  `json:"recommendations"`
	Confidence      float64   `json:"confidence"`
	CreatedBy       *string   `json:"createdBy,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	// Source marks where the failure record originates. rca.reports has no
	// severity/status/work-order columns, so rather than fabricate them the
	// asset profile exposes this marker (plus confidence + createdAt) and lets
	// the frontend stop hardcoding those fields silently.
	Source string `json:"source"`
}

// Gap represents a compliance gap
type Gap struct {
	ID          string    `json:"id"`
	GapType     string    `json:"gapType"`
	Description string    `json:"description"`
	Severity    string    `json:"severity"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Document represents a document entity linked via graph entities
type Document struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	DocumentType string    `json:"documentType"`
	AccessLevel  string    `json:"accessLevel"`
	Sensitivity  string    `json:"sensitivity"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
}

// QueryRequest represents the body of /api/copilot/query
type QueryRequest struct {
	Question       string                 `json:"question" binding:"required"`
	PlantID        string                 `json:"plantId"`
	UserID         string                 `json:"userId,omitempty"`
	OrganizationID string                 `json:"organizationId,omitempty"`
	UserRole       string                 `json:"userRole,omitempty"`
	Filters        map[string]interface{} `json:"filters"`
	AccessContext  map[string]interface{} `json:"accessContext,omitempty"`
}

// RCAGenerateReq represents the body of /api/rca/generate
type RCAGenerateReq struct {
	AssetTag           string `json:"assetTag" binding:"required"`
	FailureDescription string `json:"failureDescription" binding:"required"`
	PlantID            string `json:"plantId,omitempty"`
	UserID             string `json:"userId,omitempty"`
	OrganizationID     string `json:"organizationId,omitempty"`
}

// ComplianceGap represents the gap record returned from compliance.gaps
type ComplianceGap struct {
	ID                 string    `json:"id"`
	OrganizationID     string    `json:"organizationId"`
	PlantID            *string   `json:"plantId"`
	AssetID            *string   `json:"assetId"`
	AssetTag           *string   `json:"assetTag"`
	RequirementID      *string   `json:"requirementId"`
	GapType            string    `json:"gapType"`
	Description        string    `json:"description"`
	Severity           *string   `json:"severity"`
	EvidenceDocumentID *string   `json:"evidenceDocumentId"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// ReportJobReq represents the body of /api/reports
type ReportJobReq struct {
	PlantID    string                 `json:"plantId"`
	ReportType string                 `json:"reportType" binding:"required"`
	Format     string                 `json:"format"`
	Parameters map[string]interface{} `json:"parameters"`
}

type reportTable struct {
	Title   string
	Headers []string
	Rows    [][]string
}

const aiServiceTimeout = 12 * time.Second
// LLM-backed endpoints (copilot, RCA) run an intent classification, generation, and up
// to three citation-validation passes; 12s routinely tripped the timeout and dropped
// real answers to the fallback while the AI kept writing rag.queries.
const aiLLMServiceTimeout = 90 * time.Second
const processingRetryBaseDelay = time.Minute
const processingRetryMaxDelay = 30 * time.Minute

type aiServiceResponse struct {
	StatusCode int
	Body       []byte
}

type rateLimitEntry struct {
	Count   int
	ResetAt time.Time
}

type slidingWindowLimiter struct {
	mu      sync.Mutex
	entries map[string]rateLimitEntry
	limit   int
	window  time.Duration
	now     func() time.Time
}

func main() {
	log.Println("Starting PlantBrainAI API service...")

	// 1. Load configuration
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://plantbrain:plantbrain@localhost:5432/plantbrain"
	}

	aiServiceURL := os.Getenv("AI_SERVICE_URL")
	if aiServiceURL == "" {
		aiServiceURL = "http://localhost:8000"
	}

	// 2. Initialize PostgreSQL connection pool
	var dbPool *pgxpool.Pool
	var err error
	for i := 1; i <= 5; i++ {
		dbPool, err = pgxpool.New(context.Background(), dbURL)
		if err == nil {
			// Test connection
			err = dbPool.Ping(context.Background())
			if err == nil {
				log.Println("Successfully connected to PostgreSQL")
				break
			}
		}
		log.Printf("Failed to connect to PostgreSQL (attempt %d/5): %v. Retrying in 2s...", i, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Fatal: Failed to connect to PostgreSQL after 5 attempts: %v", err)
	}
	defer dbPool.Close()

	// Seed database with dev session and credentials only on explicit opt-in.
	// Gating on APP_ENV != production shipped a known 30-day credential to any
	// deployment that forgot to set the env var.
	if os.Getenv("SEED_DEV_DATA") == "true" {
		seedDevelopmentData(dbPool)
	}

	// 3. Initialize MinIO Client
	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:9000"
	}
	useSSL := false
	if strings.HasPrefix(endpoint, "https://") {
		endpoint = strings.TrimPrefix(endpoint, "https://")
		useSSL = true
	} else if strings.HasPrefix(endpoint, "http://") {
		endpoint = strings.TrimPrefix(endpoint, "http://")
		useSSL = false
	}

	accessKeyID := os.Getenv("S3_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("S3_SECRET_ACCESS_KEY")
	if accessKeyID == "" || secretAccessKey == "" {
		log.Fatal("Fatal: S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY must be configured")
	}

	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		log.Fatalf("Fatal: Failed to initialize MinIO client: %v", err)
	}

	// Presigning client bound to a browser-reachable endpoint. Presigned URLs are
	// signed for a specific host; the in-network `minio:9000` host does not resolve
	// from a user's browser, so downloads must be signed against S3_PUBLIC_ENDPOINT.
	publicEndpoint := os.Getenv("S3_PUBLIC_ENDPOINT")
	publicUseSSL := useSSL
	if publicEndpoint == "" {
		publicEndpoint = endpoint
		publicUseSSL = useSSL
	} else {
		if strings.HasPrefix(publicEndpoint, "https://") {
			publicEndpoint = strings.TrimPrefix(publicEndpoint, "https://")
			publicUseSSL = true
		} else if strings.HasPrefix(publicEndpoint, "http://") {
			publicEndpoint = strings.TrimPrefix(publicEndpoint, "http://")
			publicUseSSL = false
		}
	}
	publicMinioClient, err := minio.New(publicEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: publicUseSSL,
	})
	if err != nil {
		log.Fatalf("Fatal: Failed to initialize public MinIO client: %v", err)
	}

	// Ensure bucket exists
	bucketName := os.Getenv("STORAGE_BUCKET")
	if bucketName == "" {
		bucketName = "plantbrain-documents"
	}
	ctx := context.Background()
	exists, err := minioClient.BucketExists(ctx, bucketName)
	if err == nil && !exists {
		err = minioClient.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
		if err != nil {
			log.Printf("Warning: Failed to create bucket %s: %v", bucketName, err)
		} else {
			log.Printf("Successfully created bucket %s", bucketName)
		}
	} else if err != nil {
		log.Printf("Warning: Failed to check bucket existence: %v", err)
	}

	// 4. Setup Gin Router
	r := gin.Default()
	r.Use(RequestContextMiddleware())

	// CORS Middleware
	r.Use(func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowedOrigins := strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",")
		if len(allowedOrigins) == 1 && strings.TrimSpace(allowedOrigins[0]) == "" {
			allowedOrigins = []string{"http://localhost:3000"}
		}
		for _, allowedOrigin := range allowedOrigins {
			if origin != "" && origin == strings.TrimSpace(allowedOrigin) {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Set("Vary", "Origin")
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
				break
			}
		}
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Parse rate-limiting configurations
	globalRateLimit := 300
	if val, err := strconv.Atoi(os.Getenv("API_RATE_LIMIT")); err == nil {
		globalRateLimit = val
	}
	globalRateLimitWindow := time.Minute
	if val, err := time.ParseDuration(os.Getenv("API_RATE_LIMIT_WINDOW")); err == nil {
		globalRateLimitWindow = val
	}

	authRateLimit := 120
	if val, err := strconv.Atoi(os.Getenv("API_AUTH_RATE_LIMIT")); err == nil {
		authRateLimit = val
	}
	authRateLimitWindow := time.Minute
	if val, err := time.ParseDuration(os.Getenv("API_AUTH_RATE_LIMIT_WINDOW")); err == nil {
		authRateLimitWindow = val
	}

	r.Use(RateLimitMiddleware(globalRateLimit, globalRateLimitWindow))

	// Auth Middleware
	authGroup := r.Group("/api")
	authGroup.Use(AuthMiddleware(dbPool))
	authGroup.Use(RateLimitMiddleware(authRateLimit, authRateLimitWindow))

	// Endpoints
	authGroup.GET("/me", handleGetMe())
	authGroup.GET("/documents", RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer", "viewer"), handleGetDocuments(dbPool))
	authGroup.GET("/documents/:id", RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer", "viewer"), handleGetDocumentDetail(dbPool))
	authGroup.GET("/documents/:id/status", RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer", "viewer"), handleGetDocumentStatus(dbPool))
	authGroup.GET("/documents/:id/download", RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer", "viewer"), handleGetDocumentDownloadURL(dbPool, publicMinioClient, bucketName))
	authGroup.POST("/documents/upload", RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer"), handleDocumentUpload(dbPool, minioClient, bucketName, aiServiceURL, storageBaseURL(useSSL, endpoint)))
	authGroup.POST("/documents/:id/versions", RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer"), handleDocumentVersionUpload(dbPool, minioClient, bucketName, aiServiceURL, storageBaseURL(useSSL, endpoint)))
	authGroup.POST("/documents/:id/retry-processing", RequireRoles("admin", "plant_manager", "engineer", "compliance_officer"), handleRetryDocumentProcessing(dbPool, aiServiceURL))
	authGroup.DELETE("/documents/:id", RequireRoles("admin", "plant_manager", "engineer", "compliance_officer"), handleArchiveDocument(dbPool))
	authGroup.POST("/copilot/query", RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer", "viewer"), handleCopilotQuery(dbPool, aiServiceURL))
	authGroup.GET("/dashboard/metrics", RequireRoles("admin", "plant_manager", "engineer", "compliance_officer"), handleDashboardMetrics(dbPool))
	authGroup.GET("/assets", RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer", "viewer"), handleGetAssets(dbPool))
	authGroup.GET("/assets/:id", RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer", "viewer"), handleGetAssetByID(dbPool))
	authGroup.POST("/rca/generate", RequireRoles("admin", "plant_manager", "engineer"), handleRCAGenerate(dbPool, aiServiceURL))
	authGroup.GET("/compliance/gaps", RequireRoles("admin", "plant_manager", "engineer", "compliance_officer"), handleGetComplianceGaps(dbPool))
	authGroup.PATCH("/compliance/gaps/:id", RequireRoles("admin", "plant_manager", "engineer", "compliance_officer"), handleUpdateComplianceGap(dbPool))
	authGroup.POST("/compliance/scan", RequireRoles("admin", "plant_manager", "compliance_officer"), handleComplianceScan(dbPool, aiServiceURL))
	authGroup.GET("/graph", RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer", "viewer"), handleGetGraph(dbPool))
	authGroup.GET("/reports", RequireRoles("admin", "plant_manager", "engineer", "compliance_officer"), handleListReportJobs(dbPool))
	authGroup.POST("/reports", RequireRoles("admin", "plant_manager", "engineer", "compliance_officer"), handleCreateReportJob(dbPool))
	authGroup.GET("/reports/:id/download", RequireRoles("admin", "plant_manager", "engineer", "compliance_officer"), handleDownloadReport(dbPool))

	// Health Check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy", "service": "api"})
	})

	log.Printf("API Server listening on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Fatal: Server failed to start: %v", err)
	}
}

func RequestContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if requestID == "" || len(requestID) > 128 {
			requestID = uuid.NewString()
		}

		start := time.Now()
		c.Set("requestID", requestID)
		c.Writer.Header().Set("X-Request-ID", requestID)

		c.Next()

		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		log.Printf(
			"request_id=%s method=%s path=%s status=%d latency_ms=%d user_id=%s org_id=%s role=%s",
			requestID,
			c.Request.Method,
			path,
			c.Writer.Status(),
			time.Since(start).Milliseconds(),
			c.GetString("userID"),
			c.GetString("orgID"),
			c.GetString("role"),
		)
	}
}

func RateLimitMiddleware(limit int, window time.Duration) gin.HandlerFunc {
	limiter := newSlidingWindowLimiter(limit, window)
	return func(c *gin.Context) {
		key := c.GetString("userID")
		if key == "" {
			key = c.ClientIP()
		}
		if key == "" {
			key = "unknown"
		}

		allowed, retryAfter := limiter.Allow(key)
		if !allowed {
			c.Header("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":           "too many requests, please retry shortly",
				"retryAfterSec":   int(retryAfter.Seconds()),
				"requestId":       c.GetString("requestID"),
				"rateLimitWindow": window.String(),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func newSlidingWindowLimiter(limit int, window time.Duration) *slidingWindowLimiter {
	return &slidingWindowLimiter{
		entries: make(map[string]rateLimitEntry),
		limit:   limit,
		window:  window,
		now:     time.Now,
	}
}

func (l *slidingWindowLimiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	entry, ok := l.entries[key]
	if !ok || !now.Before(entry.ResetAt) {
		l.entries[key] = rateLimitEntry{Count: 1, ResetAt: now.Add(l.window)}
		l.cleanupExpired(now)
		return true, 0
	}

	if entry.Count >= l.limit {
		return false, entry.ResetAt.Sub(now)
	}

	entry.Count++
	l.entries[key] = entry
	return true, 0
}

func (l *slidingWindowLimiter) cleanupExpired(now time.Time) {
	if len(l.entries) < 1000 {
		return
	}
	for key, entry := range l.entries {
		if !now.Before(entry.ResetAt) {
			delete(l.entries, key)
		}
	}
}

// AuthMiddleware extracts session token, verifies it, and sets auth context variables.
func AuthMiddleware(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var token string

		// 1. Try Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
				token = parts[1]
			}
		}

		// 2. Try Cookie. BetterAuth names it "better-auth.session_token" (underscore);
		// the hyphenated form is kept as a fallback for older/custom clients.
		if token == "" {
			for _, name := range []string{"better-auth.session_token", "better-auth.session-token"} {
				if cookie, err := c.Cookie(name); err == nil && cookie != "" {
					token = cookie
					break
				}
			}
		}

		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: missing session token"})
			c.Abort()
			return
		}

		ctx := c.Request.Context()
		var userIDStr string
		var orgIDStr string
		var plantIDStr *string
		var roleStr *string

		// Try JWT first
		if len(strings.Split(token, ".")) == 3 {
			jwtSecret := os.Getenv("JWT_SECRET")
			if jwtSecret != "" {
				claims, err := parseJWTHS256(token, jwtSecret)
				if err == nil {
					if id, ok := claims["sub"].(string); ok {
						userIDStr = id
					} else if id, ok := claims["userId"].(string); ok {
						userIDStr = id
					}

					if org, ok := claims["orgId"].(string); ok {
						orgIDStr = org
					}
					if plant, ok := claims["plantId"].(string); ok {
						plantIDStr = &plant
					}
					if role, ok := claims["role"].(string); ok {
						roleStr = &role
					}
				} else {
					log.Printf("JWT verification failed: %v", err)
				}
			}
		}

		// Fallback to session token in identity.sessions
		if userIDStr == "" {
			// BetterAuth signs the session cookie as "<token>.<hmac-signature>";
			// the stored session row holds only the token, so use the part before
			// the first dot. Plain bearer tokens (no dot) pass through unchanged.
			sessionToken := token
			if idx := strings.IndexByte(sessionToken, '.'); idx >= 0 {
				sessionToken = sessionToken[:idx]
			}

			query := `
				SELECT
					s.user_id::text,
					u.organization_id::text,
					m.plant_id::text,
					r.name as role
				FROM identity.sessions s
				JOIN identity.users u ON s.user_id = u.id
				LEFT JOIN identity.memberships m ON u.id = m.user_id
				LEFT JOIN identity.roles r ON m.role_id = r.id
				WHERE s.token = $1 AND s.expires_at > NOW()
				LIMIT 1
			`

			err := dbPool.QueryRow(ctx, query, sessionToken).Scan(&userIDStr, &orgIDStr, &plantIDStr, &roleStr)
			if err != nil {
				log.Printf("Auth check failed: %v", err)
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: invalid or expired session"})
				c.Abort()
				return
			}
		}

		// Set variables in context
		c.Set("userID", userIDStr)
		c.Set("orgID", orgIDStr)
		if plantIDStr != nil {
			c.Set("plantID", *plantIDStr)
		} else {
			c.Set("plantID", "")
		}
		if roleStr != nil {
			c.Set("role", *roleStr)
		} else {
			c.Set("role", "")
		}

		c.Next()
	}
}

func parseJWTHS256(tokenString, secret string) (map[string]interface{}, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	headerPayload := parts[0] + "." + parts[1]

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		signature, err = base64.URLEncoding.DecodeString(parts[2])
		if err != nil {
			return nil, err
		}
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(headerPayload))
	expectedSignature := mac.Sum(nil)

	if !hmac.Equal(signature, expectedSignature) {
		return nil, fmt.Errorf("invalid signature")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payloadBytes, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, err
		}
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, err
	}

	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, fmt.Errorf("token expired")
		}
	}

	return claims, nil
}

// RequireRoles enforces coarse-grained RBAC after AuthMiddleware has populated the request context.
func RequireRoles(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if !roleAllowed(role, allowedRoles...) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":        "forbidden: role is not allowed for this action",
				"requiredRole": allowedRoles,
				"currentRole":  role,
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

func roleAllowed(role string, allowedRoles ...string) bool {
	normalizedRole := normalizeRole(role)
	if normalizedRole == "" {
		return false
	}
	for _, allowed := range allowedRoles {
		if normalizedRole == normalizeRole(allowed) {
			return true
		}
	}
	return false
}

func normalizeRole(role string) string {
	normalized := strings.ToLower(strings.TrimSpace(role))
	normalized = strings.ReplaceAll(normalized, "-", " ")
	normalized = strings.ReplaceAll(normalized, "_", " ")
	normalized = strings.Join(strings.Fields(normalized), " ")

	switch normalized {
	case "admin", "administrator":
		return "admin"
	case "engineer", "maintenance engineer", "reliability engineer":
		return "engineer"
	case "technician", "field technician", "plant operator", "operator":
		return "technician"
	case "compliance officer", "compliance":
		return "compliance_officer"
	case "plant manager", "manager":
		return "plant_manager"
	case "viewer", "read only", "read only access":
		return "viewer"
	default:
		return strings.ReplaceAll(normalized, " ", "_")
	}
}

type documentAccessPolicy struct {
	AccessLevel  string
	Sensitivity  string
	AllowedRoles []string
}

func normalizeDocumentAccessLevel(accessLevel string) string {
	normalized := strings.ToLower(strings.TrimSpace(accessLevel))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")

	switch normalized {
	case "", "default":
		return ""
	case "public", "shared":
		return "public"
	case "internal", "plant", "plant_internal":
		return "internal"
	case "restricted", "role_restricted", "role_based":
		return "restricted"
	case "confidential", "sensitive":
		return "confidential"
	default:
		return ""
	}
}

func normalizeDocumentSensitivity(sensitivity string) string {
	normalized := strings.ToLower(strings.TrimSpace(sensitivity))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")

	switch normalized {
	case "", "default":
		return ""
	case "standard", "normal", "low":
		return "standard"
	case "sensitive", "restricted":
		return "sensitive"
	case "confidential":
		return "confidential"
	case "safety_critical", "safety", "critical":
		return "safety_critical"
	default:
		return ""
	}
}

func defaultDocumentAccessPolicy(documentType string) documentAccessPolicy {
	docType := strings.ToLower(strings.TrimSpace(documentType))
	docType = strings.ReplaceAll(docType, "-", " ")
	docType = strings.ReplaceAll(docType, "_", " ")

	policy := documentAccessPolicy{
		AccessLevel: "internal",
		Sensitivity: "standard",
	}

	switch {
	case strings.Contains(docType, "compliance") ||
		strings.Contains(docType, "regulatory") ||
		strings.Contains(docType, "audit"):
		policy.AccessLevel = "restricted"
		policy.Sensitivity = "sensitive"
		policy.AllowedRoles = []string{"admin", "plant_manager", "compliance_officer"}
	case strings.Contains(docType, "incident") ||
		strings.Contains(docType, "near miss") ||
		strings.Contains(docType, "near-miss") ||
		strings.Contains(docType, "rca") ||
		strings.Contains(docType, "failure"):
		policy.AccessLevel = "restricted"
		policy.Sensitivity = "sensitive"
		policy.AllowedRoles = []string{"admin", "plant_manager", "engineer", "compliance_officer"}
	}

	return policy
}

func parseAllowedDocumentRoles(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var values []string
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return nil, fmt.Errorf("allowedRoles must be a comma-separated list or JSON array")
		}
	} else {
		values = strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == ';' || r == '\n'
		})
	}

	seen := map[string]bool{}
	roles := []string{}
	for _, value := range values {
		role := normalizeRole(value)
		if role == "" || seen[role] {
			continue
		}
		seen[role] = true
		roles = append(roles, role)
	}
	return roles, nil
}

func canSetSensitiveDocumentPolicy(role string) bool {
	return roleAllowed(role, "admin", "plant_manager", "engineer", "compliance_officer")
}

func requestedSensitiveDocumentPolicy(accessLevel, sensitivity, allowedRoles string) bool {
	return strings.TrimSpace(accessLevel) != "" ||
		strings.TrimSpace(sensitivity) != "" ||
		strings.TrimSpace(allowedRoles) != ""
}

func buildDocumentAccessPolicy(documentType, requestedAccessLevel, requestedSensitivity, requestedAllowedRoles, uploaderRole string) (documentAccessPolicy, error) {
	policy := defaultDocumentAccessPolicy(documentType)

	accessLevel := strings.TrimSpace(requestedAccessLevel)
	if accessLevel != "" {
		normalizedAccessLevel := normalizeDocumentAccessLevel(accessLevel)
		if normalizedAccessLevel == "" {
			return documentAccessPolicy{}, fmt.Errorf("invalid accessLevel")
		}
		policy.AccessLevel = normalizedAccessLevel
	}

	sensitivity := strings.TrimSpace(requestedSensitivity)
	if sensitivity != "" {
		normalizedSensitivity := normalizeDocumentSensitivity(sensitivity)
		if normalizedSensitivity == "" {
			return documentAccessPolicy{}, fmt.Errorf("invalid sensitivity")
		}
		policy.Sensitivity = normalizedSensitivity
	}

	allowedRoles, err := parseAllowedDocumentRoles(requestedAllowedRoles)
	if err != nil {
		return documentAccessPolicy{}, err
	}
	if allowedRoles != nil {
		policy.AllowedRoles = allowedRoles
	}

	if requestedSensitiveDocumentPolicy(requestedAccessLevel, requestedSensitivity, requestedAllowedRoles) && !canSetSensitiveDocumentPolicy(uploaderRole) {
		return documentAccessPolicy{}, fmt.Errorf("current role cannot set document access policy")
	}

	if (policy.AccessLevel == "restricted" || policy.AccessLevel == "confidential") && len(policy.AllowedRoles) == 0 {
		policy.AllowedRoles = []string{"admin"}
	}

	// documents.allowed_roles is NOT NULL (default '{}'); a nil slice would be sent as
	// NULL and violate the constraint, so normalize to an empty slice.
	if policy.AllowedRoles == nil {
		policy.AllowedRoles = []string{}
	}

	return policy, nil
}

func documentReadableByRole(role, accessLevel string, allowedRoles []string) bool {
	normalizedRole := normalizeRole(role)
	if normalizedRole == "" {
		return false
	}
	if normalizedRole == "admin" {
		return true
	}

	normalizedAccessLevel := normalizeDocumentAccessLevel(accessLevel)
	if normalizedAccessLevel == "" {
		normalizedAccessLevel = "internal"
	}

	switch normalizedAccessLevel {
	case "public", "internal":
		return true
	case "restricted", "confidential":
		return roleAllowed(normalizedRole, allowedRoles...)
	default:
		return false
	}
}

func requireDocumentAccess(c *gin.Context, accessLevel string, allowedRoles []string) bool {
	if documentReadableByRole(c.GetString("role"), accessLevel, allowedRoles) {
		return true
	}

	normalizedAccessLevel := normalizeDocumentAccessLevel(accessLevel)
	if normalizedAccessLevel == "" {
		normalizedAccessLevel = "internal"
	}

	c.JSON(http.StatusForbidden, gin.H{
		"error":       "forbidden: document is restricted for this role",
		"accessLevel": normalizedAccessLevel,
		"currentRole": normalizeRole(c.GetString("role")),
	})
	c.Abort()
	return false
}

func documentSourceRestricted(accessLevel string, allowedRoles []string) bool {
	normalizedAccessLevel := normalizeDocumentAccessLevel(accessLevel)
	if normalizedAccessLevel == "" {
		normalizedAccessLevel = "internal"
	}
	return normalizedAccessLevel == "restricted" || normalizedAccessLevel == "confidential" || len(allowedRoles) > 0
}

type DocumentSourcePolicy struct {
	ReadableAccessLevels []string
	AllowedRoles         []string
}

func documentSourcePolicyForRole(role string) DocumentSourcePolicy {
	normalizedRole := normalizeRole(role)
	if normalizedRole == "admin" {
		return DocumentSourcePolicy{
			ReadableAccessLevels: []string{"public", "internal", "restricted", "confidential"},
			AllowedRoles:         []string{},
		}
	}
	return DocumentSourcePolicy{
		ReadableAccessLevels: []string{"public", "internal"},
		AllowedRoles:         []string{normalizedRole},
	}
}

func documentAccessContextForRole(role string) map[string]interface{} {
	normalizedRole := normalizeRole(role)
	return map[string]interface{}{
		"role":                 normalizedRole,
		"isAdmin":              normalizedRole == "admin",
		"readableAccessLevels": []string{"public", "internal"},
		"allowedRoles":         []string{normalizedRole},
	}
}

// GET /api/me
func handleGetMe() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"userId":  c.GetString("userID"),
			"orgId":   c.GetString("orgID"),
			"plantId": c.GetString("plantID"),
			"role":    c.GetString("role"),
		})
	}
}

// GET /api/documents
func handleGetDocuments(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("orgID")
		plantID := c.Query("plantId")
		if plantID == "" {
			plantID = c.GetString("plantID")
		}
		if plantID != "" && !requirePlantAccess(c, dbPool, plantID) {
			return
		}

		ctx := c.Request.Context()
		role := normalizeRole(c.GetString("role"))
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
				"sourceRestricted":         documentSourceRestricted(accessLevel, allowedRoles),
				"sourceDownloadAllowed":    documentReadableByRole(role, accessLevel, allowedRoles),
				"status":                   status,
				"ocrConfidence":            ocrConfidence,
				"classificationConfidence": classificationConfidence,
				"createdAt":                createdAt,
			})
		}

		c.JSON(http.StatusOK, documents)
	}
}

func nullableUUID(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func floatValue(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func jsonTextList(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}

	var values []string
	if err := json.Unmarshal(raw, &values); err == nil {
		return values
	}

	var objects []map[string]interface{}
	if err := json.Unmarshal(raw, &objects); err == nil {
		values = []string{}
		for _, item := range objects {
			for _, key := range []string{"event", "cause", "action", "recommendation", "description", "summary"} {
				if value, ok := item[key]; ok {
					values = append(values, fmt.Sprint(value))
					break
				}
			}
		}
		return values
	}

	var generic []interface{}
	if err := json.Unmarshal(raw, &generic); err == nil {
		values = []string{}
		for _, item := range generic {
			values = append(values, fmt.Sprint(item))
		}
		return values
	}

	return []string{string(raw)}
}

func auditMetadataJSON(c *gin.Context, metadata map[string]interface{}) []byte {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata["requestId"] = c.GetString("requestID")
	metadata["method"] = c.Request.Method
	path := c.FullPath()
	if path == "" {
		path = c.Request.URL.Path
	}
	metadata["path"] = path

	payload, err := json.Marshal(metadata)
	if err != nil {
		return []byte("{}")
	}
	return payload
}

func recordAuditEvent(c *gin.Context, dbPool *pgxpool.Pool, plantID, eventType, resourceType, resourceID string, metadata map[string]interface{}) {
	_, _ = dbPool.Exec(c.Request.Context(), `
		INSERT INTO audit.events (organization_id, plant_id, actor_user_id, event_type, resource_type, resource_id, correlation_id, metadata_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, c.GetString("orgID"), nullableUUID(plantID), nullableUUID(c.GetString("userID")), eventType, resourceType, resourceID, c.GetString("requestID"), auditMetadataJSON(c, metadata))
}

func callAIService(ctx context.Context, method, aiServiceURL, path string, body []byte) (aiServiceResponse, error) {
	return callAIServiceOpts(ctx, method, aiServiceURL, path, body, aiServiceTimeout, nil)
}

func callAIServiceOpts(ctx context.Context, method, aiServiceURL, path string, body []byte, timeout time.Duration, headers map[string]string) (aiServiceResponse, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	target := strings.TrimRight(aiServiceURL, "/") + path
	req, err := http.NewRequestWithContext(reqCtx, method, target, bytes.NewReader(body))
	if err != nil {
		return aiServiceResponse{}, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return aiServiceResponse{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return aiServiceResponse{}, err
	}

	return aiServiceResponse{StatusCode: resp.StatusCode, Body: respBody}, nil
}

// aiForwardHeaders propagates request/user identity so the AI service can correlate
// model calls (ai.model_calls.correlation_id) and apply per-user rate limiting.
func aiForwardHeaders(c *gin.Context) map[string]string {
	return map[string]string{
		"X-Request-ID": c.GetString("requestID"),
		"X-User-Id":    c.GetString("userID"),
	}
}

func aiFailureReason(err error, response aiServiceResponse) string {
	if err != nil {
		return err.Error()
	}
	if len(response.Body) == 0 {
		return fmt.Sprintf("AI service returned HTTP %d", response.StatusCode)
	}
	body := strings.TrimSpace(string(response.Body))
	if len(body) > 300 {
		body = body[:300]
	}
	return fmt.Sprintf("AI service returned HTTP %d: %s", response.StatusCode, body)
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

func requirePlantAccess(c *gin.Context, dbPool *pgxpool.Pool, plantID string) bool {
	if plantID == "" {
		return true
	}

	var allowed bool
	err := dbPool.QueryRow(c.Request.Context(), `
		SELECT EXISTS (
			SELECT 1
			FROM identity.plants p
			JOIN identity.memberships m
			  ON m.organization_id = p.organization_id
			 AND m.user_id = $3
			 AND (m.plant_id IS NULL OR m.plant_id = p.id)
			WHERE p.id = $1
			  AND p.organization_id = $2
		)
	`, plantID, c.GetString("orgID"), c.GetString("userID")).Scan(&allowed)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to verify plant access: %v", err)})
		return false
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: plant is outside the current user membership"})
		return false
	}
	return true
}

// GET /api/documents/:id
func handleGetDocumentDetail(dbPool *pgxpool.Pool) gin.HandlerFunc {
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
		if !requirePlantAccess(c, dbPool, plantID) {
			return
		}
		sourceDownloadAllowed := documentReadableByRole(c.GetString("role"), accessLevel, allowedRoles)

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
			"sourceRestricted":      documentSourceRestricted(accessLevel, allowedRoles),
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

// GET /api/documents/:id/status
func handleGetDocumentStatus(dbPool *pgxpool.Pool) gin.HandlerFunc {
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
		if !requirePlantAccess(c, dbPool, plantID) {
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

// GET /api/documents/:id/download
func handleGetDocumentDownloadURL(dbPool *pgxpool.Pool, minioClient *minio.Client, bucketName string) gin.HandlerFunc {
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
		if !requirePlantAccess(c, dbPool, plantID) {
			return
		}
		if !requireDocumentAccess(c, accessLevel, allowedRoles) {
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

// DELETE /api/documents/:id
func handleArchiveDocument(dbPool *pgxpool.Pool) gin.HandlerFunc {
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
		if !requirePlantAccess(c, dbPool, plantID) {
			return
		}
		if !requireDocumentAccess(c, accessLevel, allowedRoles) {
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

// POST /api/documents/upload
func handleDocumentUpload(dbPool *pgxpool.Pool, minioClient *minio.Client, bucketName, aiServiceURL, s3Endpoint string) gin.HandlerFunc {
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

		// FR-3 metadata supplied by the uploader. Title falls back to the filename; the
		// rest is preserved in documents.metadata_json so it is not silently dropped.
		docTitle := strings.TrimSpace(c.PostForm("title"))
		if docTitle == "" {
			docTitle = header.Filename
		}
		versionLabel := strings.TrimSpace(c.PostForm("version"))
		if versionLabel == "" {
			versionLabel = "v1"
		}
		docMetadata := map[string]interface{}{}
		if dept := strings.TrimSpace(c.PostForm("department")); dept != "" {
			docMetadata["department"] = dept
		}
		if assetTag := strings.TrimSpace(c.PostForm("assetTag")); assetTag != "" {
			docMetadata["assetTag"] = strings.ToUpper(assetTag)
		}
		docMetadataJSON, err := json.Marshal(docMetadata)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to encode document metadata: %v", err)})
			return
		}

		orgID := c.GetString("orgID")
		userID := c.GetString("userID")
		accessPolicy, err := buildDocumentAccessPolicy(
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
		if !requirePlantAccess(c, dbPool, plantID) {
			return
		}

		// Create local target path. In Docker this directory is shared with the AI service.
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

		// Save file locally and compute hash
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

		// Upload to MinIO
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

		// Insert metadata into database within a transaction
		ctx := c.Request.Context()
		tx, err := dbPool.Begin(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to start database transaction: %v", err)})
			return
		}
		defer tx.Rollback(ctx)

		// 1. Insert into document.documents
		_, err = tx.Exec(ctx, `
			INSERT INTO document.documents (
				id, organization_id, plant_id, title, document_type, access_level, sensitivity, allowed_roles,
				current_version_id, status, uploaded_by, metadata_json, created_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULL, 'UPLOADED', $9, $10, NOW(), NOW())
		`, docID, orgID, plantID, docTitle, documentType, accessPolicy.AccessLevel, accessPolicy.Sensitivity, accessPolicy.AllowedRoles, userID, docMetadataJSON)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to insert document metadata: %v", err)})
			return
		}

		// 2. Insert into document.document_versions
		_, err = tx.Exec(ctx, `
			INSERT INTO document.document_versions (id, document_id, version_label, file_url, file_type, file_sha256, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, NOW())
		`, versionID, docID, versionLabel, fileURL, strings.TrimPrefix(ext, "."), fileSHA256)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to insert document version: %v", err)})
			return
		}

		// 3. Update documents to point to current_version_id
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

		// Trigger Python AI Service ingestion pipeline asynchronously (HTTP POST)
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

// POST /api/documents/:id/versions
func handleDocumentVersionUpload(dbPool *pgxpool.Pool, minioClient *minio.Client, bucketName, aiServiceURL, s3Endpoint string) gin.HandlerFunc {
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
		if !requirePlantAccess(c, dbPool, plantID) {
			return
		}
		if !requireDocumentAccess(c, accessLevel, allowedRoles) {
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
			"fileSha256":   fileSHA256,
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

// POST /api/documents/:id/retry-processing
func handleRetryDocumentProcessing(dbPool *pgxpool.Pool, aiServiceURL string) gin.HandlerFunc {
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
		if !requirePlantAccess(c, dbPool, plantID) {
			return
		}
		if !requireDocumentAccess(c, accessLevel, allowedRoles) {
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

// POST /api/copilot/query
func handleCopilotQuery(dbPool *pgxpool.Pool, aiServiceURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req QueryRequest
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
		req.UserRole = normalizeRole(c.GetString("role"))
		req.AccessContext = documentAccessContextForRole(c.GetString("role"))
		if req.Filters == nil {
			req.Filters = map[string]interface{}{}
		}
		req.Filters["documentAccess"] = req.AccessContext

		if req.PlantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "plantId is required"})
			return
		}
		if !requirePlantAccess(c, dbPool, req.PlantID) {
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

		aiResp, err := callAIServiceOpts(c.Request.Context(), http.MethodPost, aiServiceURL, "/query", payloadBytes, aiLLMServiceTimeout, aiForwardHeaders(c))
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

// GET /api/dashboard/metrics
func handleDashboardMetrics(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("orgID")
		plantID := c.Query("plantId")
		if plantID == "" {
			plantID = c.GetString("plantID")
		}
		if plantID != "" && !requirePlantAccess(c, dbPool, plantID) {
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

// GET /api/assets
func handleGetAssets(dbPool *pgxpool.Pool) gin.HandlerFunc {
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
			if !requirePlantAccess(c, dbPool, plantID) {
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

		assets := []Asset{}
		for rows.Next() {
			var a Asset
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

// GET /api/assets/:id
func handleGetAssetByID(dbPool *pgxpool.Pool) gin.HandlerFunc {
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

		var a Asset
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
		if !requirePlantAccess(c, dbPool, a.PlantID) {
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
		failures := []Failure{}
		fRows, err := dbPool.Query(ctx, `
			SELECT id::text, failure_summary, timeline, probable_causes, recommendations, confidence, created_by::text, created_at
			FROM rca.reports
			WHERE asset_id = $1 AND organization_id = $2
			ORDER BY created_at DESC
		`, a.ID, orgID)
		if err == nil {
			defer fRows.Close()
			for fRows.Next() {
				var f Failure
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
		gaps := []Gap{}
		gRows, err := dbPool.Query(ctx, `
			SELECT id, gap_type, description, severity, status, created_at, updated_at
			FROM compliance.gaps
			WHERE asset_id = $1 AND organization_id = $2
			ORDER BY created_at DESC
		`, a.ID, orgID)
		if err == nil {
			defer gRows.Close()
			for gRows.Next() {
				var g Gap
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
		documents := []Document{}
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
				var d Document
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

// POST /api/rca/generate
func handleRCAGenerate(dbPool *pgxpool.Pool, aiServiceURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RCAGenerateReq
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
		if !requirePlantAccess(c, dbPool, req.PlantID) {
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

		aiResp, err := callAIServiceOpts(c.Request.Context(), http.MethodPost, aiServiceURL, "/rca", payloadBytes, aiLLMServiceTimeout, aiForwardHeaders(c))
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

// GET /api/compliance/gaps
func handleGetComplianceGaps(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("orgID")
		plantID := c.Query("plantId")
		if plantID == "" {
			plantID = c.GetString("plantID")
		}
		if plantID != "" && !requirePlantAccess(c, dbPool, plantID) {
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

		gaps := []ComplianceGap{}
		for rows.Next() {
			var g ComplianceGap
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

// PATCH /api/compliance/gaps/:id
func handleUpdateComplianceGap(dbPool *pgxpool.Pool) gin.HandlerFunc {
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

		// Resolve the gap's plant first so tenancy is enforced with the same
		// membership check used elsewhere. plant_id may be NULL on org-wide gaps,
		// in which case requirePlantAccess("") short-circuits to allowed.
		var plantID *string
		if err := dbPool.QueryRow(ctx, `
			SELECT plant_id::text
			FROM compliance.gaps
			WHERE id = $1 AND organization_id = $2
		`, gapID, orgID).Scan(&plantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "compliance gap not found"})
			return
		}
		if !requirePlantAccess(c, dbPool, stringValue(plantID)) {
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

// POST /api/compliance/scan
func handleComplianceScan(dbPool *pgxpool.Pool, aiServiceURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		plantID := c.Query("plantId")
		if plantID == "" {
			plantID = c.GetString("plantID")
		}
		if plantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "plantId is required"})
			return
		}
		if !requirePlantAccess(c, dbPool, plantID) {
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

// GET /api/graph
func handleGetGraph(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("orgID")
		plantID := c.Query("plantId")
		if plantID == "" {
			plantID = c.GetString("plantID")
		}
		if plantID != "" && !requirePlantAccess(c, dbPool, plantID) {
			return
		}
		sourcePolicy := documentSourcePolicyForRole(c.GetString("role"))

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

// GET /api/reports
func handleListReportJobs(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID := c.GetString("orgID")
		plantID := c.Query("plantId")
		if plantID == "" {
			plantID = c.GetString("plantID")
		}
		if plantID != "" && !requirePlantAccess(c, dbPool, plantID) {
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

// POST /api/reports
func handleCreateReportJob(dbPool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ReportJobReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		orgID := c.GetString("orgID")
		userID := c.GetString("userID")

		if req.PlantID == "" {
			req.PlantID = c.GetString("plantID")
		}
		if req.PlantID != "" && !requirePlantAccess(c, dbPool, req.PlantID) {
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

// GET /api/reports/:id/download
func handleDownloadReport(dbPool *pgxpool.Pool) gin.HandlerFunc {
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
		if plantID != nil && !requirePlantAccess(c, dbPool, *plantID) {
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

func normalizeReportFormat(req ReportJobReq) (string, error) {
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

func reportExtension(format string) string {
	if format == "" {
		return "csv"
	}
	return format
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

func shortID(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:8]
}

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

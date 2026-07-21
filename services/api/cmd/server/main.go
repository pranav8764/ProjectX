package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"plantbrain-api/internal/handlers"
	"plantbrain-api/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

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
	r.Use(middleware.RequestContextMiddleware())

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

	r.Use(middleware.RateLimitMiddleware(globalRateLimit, globalRateLimitWindow))

	// Auth Middleware Group
	authGroup := r.Group("/api")
	authGroup.Use(middleware.AuthMiddleware(dbPool))
	authGroup.Use(middleware.RateLimitMiddleware(authRateLimit, authRateLimitWindow))

	// Endpoints
	authGroup.GET("/me", handlers.HandleGetMe())
	authGroup.GET("/documents", middleware.RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer", "viewer"), handlers.HandleGetDocuments(dbPool))
	authGroup.GET("/documents/:id", middleware.RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer", "viewer"), handlers.HandleGetDocumentDetail(dbPool))
	authGroup.GET("/documents/:id/status", middleware.RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer", "viewer"), handlers.HandleGetDocumentStatus(dbPool))
	authGroup.GET("/documents/:id/download", middleware.RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer", "viewer"), handlers.HandleGetDocumentDownloadURL(dbPool, publicMinioClient, bucketName))
	authGroup.POST("/documents/upload", middleware.RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer"), handlers.HandleDocumentUpload(dbPool, minioClient, bucketName, aiServiceURL, storageBaseURL(useSSL, endpoint)))
	authGroup.POST("/documents/:id/versions", middleware.RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer"), handlers.HandleDocumentVersionUpload(dbPool, minioClient, bucketName, aiServiceURL, storageBaseURL(useSSL, endpoint)))
	authGroup.POST("/documents/:id/retry-processing", middleware.RequireRoles("admin", "plant_manager", "engineer", "compliance_officer"), handlers.HandleRetryDocumentProcessing(dbPool, aiServiceURL))
	authGroup.DELETE("/documents/:id", middleware.RequireRoles("admin", "plant_manager", "engineer", "compliance_officer"), handlers.HandleArchiveDocument(dbPool))
	authGroup.POST("/copilot/query", middleware.RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer", "viewer"), handlers.HandleCopilotQuery(dbPool, aiServiceURL))
	authGroup.GET("/dashboard/metrics", middleware.RequireRoles("admin", "plant_manager", "engineer", "compliance_officer"), handlers.HandleDashboardMetrics(dbPool))
	authGroup.GET("/assets", middleware.RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer", "viewer"), handlers.HandleGetAssets(dbPool))
	authGroup.GET("/assets/:id", middleware.RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer", "viewer"), handlers.HandleGetAssetByID(dbPool))
	authGroup.POST("/rca/generate", middleware.RequireRoles("admin", "plant_manager", "engineer"), handlers.HandleRCAGenerate(dbPool, aiServiceURL))
	authGroup.GET("/compliance/gaps", middleware.RequireRoles("admin", "plant_manager", "engineer", "compliance_officer"), handlers.HandleGetComplianceGaps(dbPool))
	authGroup.PATCH("/compliance/gaps/:id", middleware.RequireRoles("admin", "plant_manager", "engineer", "compliance_officer"), handlers.HandleUpdateComplianceGap(dbPool))
	authGroup.POST("/compliance/scan", middleware.RequireRoles("admin", "plant_manager", "compliance_officer"), handlers.HandleComplianceScan(dbPool, aiServiceURL))
	authGroup.GET("/graph", middleware.RequireRoles("admin", "plant_manager", "engineer", "technician", "compliance_officer", "viewer"), handlers.HandleGetGraph(dbPool))
	authGroup.GET("/reports", middleware.RequireRoles("admin", "plant_manager", "engineer", "compliance_officer"), handlers.HandleListReportJobs(dbPool))
	authGroup.POST("/reports", middleware.RequireRoles("admin", "plant_manager", "engineer", "compliance_officer"), handlers.HandleCreateReportJob(dbPool))
	authGroup.GET("/reports/:id/download", middleware.RequireRoles("admin", "plant_manager", "engineer", "compliance_officer"), handlers.HandleDownloadReport(dbPool))

	// Health Check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy", "service": "api"})
	})

	log.Printf("API Server listening on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Fatal: Server failed to start: %v", err)
	}
}

func storageBaseURL(useSSL bool, endpoint string) string {
	scheme := "http"
	if useSSL {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, strings.TrimRight(endpoint, "/"))
}

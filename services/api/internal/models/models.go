package models

import "time"

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

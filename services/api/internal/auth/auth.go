package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DocumentAccessPolicy represents the policy for accessing a document.
type DocumentAccessPolicy struct {
	AccessLevel  string
	Sensitivity  string
	AllowedRoles []string
}

// DocumentSourcePolicy represents the policy rules for document retrieval by role.
type DocumentSourcePolicy struct {
	ReadableAccessLevels []string
	AllowedRoles         []string
}

// RequirePlantAccess verifies if the current user has access to a specific plant.
func RequirePlantAccess(c *gin.Context, dbPool *pgxpool.Pool, plantID string) bool {
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
		c.Abort()
		return false
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: plant is outside the current user membership"})
		c.Abort()
		return false
	}
	return true
}

// RoleAllowed checks if a normalized user role is present in a list of allowed roles.
func RoleAllowed(role string, allowedRoles ...string) bool {
	normalizedRole := NormalizeRole(role)
	if normalizedRole == "" {
		return false
	}
	for _, allowed := range allowedRoles {
		if normalizedRole == NormalizeRole(allowed) {
			return true
		}
	}
	return false
}

// NormalizeRole maps various role alias strings into normalized role values.
func NormalizeRole(role string) string {
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

// NormalizeDocumentAccessLevel maps access level inputs to a clean standard form.
func NormalizeDocumentAccessLevel(accessLevel string) string {
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

// NormalizeDocumentSensitivity maps document sensitivity strings to standardized values.
func NormalizeDocumentSensitivity(sensitivity string) string {
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

// DefaultDocumentAccessPolicy returns default document access values based on type.
func DefaultDocumentAccessPolicy(documentType string) DocumentAccessPolicy {
	docType := strings.ToLower(strings.TrimSpace(documentType))
	docType = strings.ReplaceAll(docType, "-", " ")
	docType = strings.ReplaceAll(docType, "_", " ")

	policy := DocumentAccessPolicy{
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

// ParseAllowedDocumentRoles processes raw strings (comma-separated or json lists) to roles.
func ParseAllowedDocumentRoles(raw string) ([]string, error) {
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
		role := NormalizeRole(value)
		if role == "" || seen[role] {
			continue
		}
		seen[role] = true
		roles = append(roles, role)
	}
	return roles, nil
}

// CanSetSensitiveDocumentPolicy decides if the user role can set restricted policy.
func CanSetSensitiveDocumentPolicy(role string) bool {
	return RoleAllowed(role, "admin", "plant_manager", "engineer", "compliance_officer")
}

// RequestedSensitiveDocumentPolicy checks if custom policy values were requested.
func RequestedSensitiveDocumentPolicy(accessLevel, sensitivity, allowedRoles string) bool {
	return strings.TrimSpace(accessLevel) != "" ||
		strings.TrimSpace(sensitivity) != "" ||
		strings.TrimSpace(allowedRoles) != ""
}

// BuildDocumentAccessPolicy calculates final policy based on default & overrides.
func BuildDocumentAccessPolicy(documentType, requestedAccessLevel, requestedSensitivity, requestedAllowedRoles, uploaderRole string) (DocumentAccessPolicy, error) {
	policy := DefaultDocumentAccessPolicy(documentType)

	accessLevel := strings.TrimSpace(requestedAccessLevel)
	if accessLevel != "" {
		normalizedAccessLevel := NormalizeDocumentAccessLevel(accessLevel)
		if normalizedAccessLevel == "" {
			return DocumentAccessPolicy{}, fmt.Errorf("invalid accessLevel")
		}
		policy.AccessLevel = normalizedAccessLevel
	}

	sensitivity := strings.TrimSpace(requestedSensitivity)
	if sensitivity != "" {
		normalizedSensitivity := NormalizeDocumentSensitivity(sensitivity)
		if normalizedSensitivity == "" {
			return DocumentAccessPolicy{}, fmt.Errorf("invalid sensitivity")
		}
		policy.Sensitivity = normalizedSensitivity
	}

	allowedRoles, err := ParseAllowedDocumentRoles(requestedAllowedRoles)
	if err != nil {
		return DocumentAccessPolicy{}, err
	}
	if allowedRoles != nil {
		policy.AllowedRoles = allowedRoles
	}

	if RequestedSensitiveDocumentPolicy(requestedAccessLevel, requestedSensitivity, requestedAllowedRoles) && !CanSetSensitiveDocumentPolicy(uploaderRole) {
		return DocumentAccessPolicy{}, fmt.Errorf("current role cannot set document access policy")
	}

	if (policy.AccessLevel == "restricted" || policy.AccessLevel == "confidential") && len(policy.AllowedRoles) == 0 {
		policy.AllowedRoles = []string{"admin"}
	}

	return policy, nil
}

// DocumentReadableByRole asserts whether a user role can read a document.
func DocumentReadableByRole(role, accessLevel string, allowedRoles []string) bool {
	normalizedRole := NormalizeRole(role)
	if normalizedRole == "" {
		return false
	}
	if normalizedRole == "admin" {
		return true
	}

	normalizedAccessLevel := NormalizeDocumentAccessLevel(accessLevel)
	if normalizedAccessLevel == "" {
		normalizedAccessLevel = "internal"
	}

	switch normalizedAccessLevel {
	case "public", "internal":
		return true
	case "restricted", "confidential":
		return RoleAllowed(normalizedRole, allowedRoles...)
	default:
		return false
	}
}

// RequireDocumentAccess checks document readability and aborts request if forbidden.
func RequireDocumentAccess(c *gin.Context, accessLevel string, allowedRoles []string) bool {
	if DocumentReadableByRole(c.GetString("role"), accessLevel, allowedRoles) {
		return true
	}

	normalizedAccessLevel := NormalizeDocumentAccessLevel(accessLevel)
	if normalizedAccessLevel == "" {
		normalizedAccessLevel = "internal"
	}

	c.JSON(http.StatusForbidden, gin.H{
		"error":       "forbidden: document is restricted for this role",
		"accessLevel": normalizedAccessLevel,
		"currentRole": NormalizeRole(c.GetString("role")),
	})
	c.Abort()
	return false
}

// DocumentSourceRestricted checks if a document has access restrictions.
func DocumentSourceRestricted(accessLevel string, allowedRoles []string) bool {
	normalizedAccessLevel := NormalizeDocumentAccessLevel(accessLevel)
	if normalizedAccessLevel == "" {
		normalizedAccessLevel = "internal"
	}
	return normalizedAccessLevel == "restricted" || normalizedAccessLevel == "confidential" || len(allowedRoles) > 0
}

// DocumentSourcePolicyForRole defines standard document filtering policies per role.
func DocumentSourcePolicyForRole(role string) DocumentSourcePolicy {
	normalizedRole := NormalizeRole(role)
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

// DocumentAccessContextForRole returns query filter mapping for vector databases.
func DocumentAccessContextForRole(role string) map[string]interface{} {
	normalizedRole := NormalizeRole(role)
	return map[string]interface{}{
		"role":                 normalizedRole,
		"isAdmin":              normalizedRole == "admin",
		"readableAccessLevels": []string{"public", "internal"},
		"allowedRoles":         []string{normalizedRole},
	}
}

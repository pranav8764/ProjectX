package main

import "testing"

func TestNormalizeRoleAliases(t *testing.T) {
	tests := map[string]string{
		"Admin":                "admin",
		"administrator":        "admin",
		"Engineer":             "engineer",
		"Maintenance Engineer": "engineer",
		"Reliability Engineer": "engineer",
		"Field Technician":     "technician",
		"Plant Operator":       "technician",
		"Compliance Officer":   "compliance_officer",
		"Plant Manager":        "plant_manager",
		"Viewer":               "viewer",
		"read-only":            "viewer",
	}

	for input, expected := range tests {
		if actual := normalizeRole(input); actual != expected {
			t.Fatalf("normalizeRole(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestDefaultDocumentAccessPolicy(t *testing.T) {
	tests := []struct {
		name             string
		documentType     string
		wantAccessLevel  string
		wantSensitivity  string
		wantAllowedRoles []string
	}{
		{
			name:            "sop stays available to plant users",
			documentType:    "SOP",
			wantAccessLevel: "internal",
			wantSensitivity: "standard",
		},
		{
			name:             "compliance documents are restricted",
			documentType:     "Compliance Document",
			wantAccessLevel:  "restricted",
			wantSensitivity:  "sensitive",
			wantAllowedRoles: []string{"admin", "plant_manager", "compliance_officer"},
		},
		{
			name:             "incident reports include engineers",
			documentType:     "Incident Report",
			wantAccessLevel:  "restricted",
			wantSensitivity:  "sensitive",
			wantAllowedRoles: []string{"admin", "plant_manager", "engineer", "compliance_officer"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := defaultDocumentAccessPolicy(test.documentType)
			if got.AccessLevel != test.wantAccessLevel {
				t.Fatalf("AccessLevel = %q, want %q", got.AccessLevel, test.wantAccessLevel)
			}
			if got.Sensitivity != test.wantSensitivity {
				t.Fatalf("Sensitivity = %q, want %q", got.Sensitivity, test.wantSensitivity)
			}
			if !stringSlicesEqual(got.AllowedRoles, test.wantAllowedRoles) {
				t.Fatalf("AllowedRoles = %v, want %v", got.AllowedRoles, test.wantAllowedRoles)
			}
		})
	}
}

func TestBuildDocumentAccessPolicy(t *testing.T) {
	policy, err := buildDocumentAccessPolicy("Audit Report", "", "", "", "Technician")
	if err != nil {
		t.Fatalf("inferred policy should not require elevated role: %v", err)
	}
	if policy.AccessLevel != "restricted" || policy.Sensitivity != "sensitive" {
		t.Fatalf("policy = %+v, want restricted sensitive policy", policy)
	}

	policy, err = buildDocumentAccessPolicy("Manual", "restricted", "sensitive", "Engineer, Compliance Officer", "Plant Manager")
	if err != nil {
		t.Fatalf("plant manager override returned error: %v", err)
	}
	if policy.AccessLevel != "restricted" || policy.Sensitivity != "sensitive" {
		t.Fatalf("policy = %+v, want restricted sensitive policy", policy)
	}
	if !stringSlicesEqual(policy.AllowedRoles, []string{"engineer", "compliance_officer"}) {
		t.Fatalf("AllowedRoles = %v, want normalized roles", policy.AllowedRoles)
	}

	if _, err := buildDocumentAccessPolicy("Manual", "restricted", "", "Engineer", "Technician"); err == nil {
		t.Fatalf("technician explicit restricted override should be denied")
	}
}

func TestDocumentReadableByRole(t *testing.T) {
	tests := []struct {
		name         string
		role         string
		accessLevel  string
		allowedRoles []string
		want         bool
	}{
		{
			name:        "technician can read internal documents",
			role:        "Field Technician",
			accessLevel: "internal",
			want:        true,
		},
		{
			name:         "technician cannot read restricted compliance document",
			role:         "Technician",
			accessLevel:  "restricted",
			allowedRoles: []string{"admin", "plant_manager", "compliance_officer"},
			want:         false,
		},
		{
			name:         "compliance officer can read restricted compliance document",
			role:         "Compliance Officer",
			accessLevel:  "restricted",
			allowedRoles: []string{"admin", "plant_manager", "compliance_officer"},
			want:         true,
		},
		{
			name:         "admin can read confidential document without explicit role",
			role:         "Admin",
			accessLevel:  "confidential",
			allowedRoles: []string{},
			want:         true,
		},
		{
			name:        "empty role is denied",
			role:        "",
			accessLevel: "internal",
			want:        false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := documentReadableByRole(test.role, test.accessLevel, test.allowedRoles)
			if got != test.want {
				t.Fatalf("documentReadableByRole(%q, %q, %v) = %t, want %t", test.role, test.accessLevel, test.allowedRoles, got, test.want)
			}
		})
	}
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func TestRoleAllowed(t *testing.T) {
	tests := []struct {
		name    string
		role    string
		allowed []string
		want    bool
	}{
		{
			name:    "maintenance engineer can use engineer gate",
			role:    "Maintenance Engineer",
			allowed: []string{"engineer"},
			want:    true,
		},
		{
			name:    "field technician cannot archive documents",
			role:    "Field Technician",
			allowed: []string{"admin", "engineer"},
			want:    false,
		},
		{
			name:    "compliance officer can export reports",
			role:    "Compliance Officer",
			allowed: []string{"admin", "plant_manager", "engineer", "compliance_officer"},
			want:    true,
		},
		{
			name:    "viewer is read only",
			role:    "Viewer",
			allowed: []string{"admin", "plant_manager"},
			want:    false,
		},
		{
			name:    "empty role is denied",
			role:    "",
			allowed: []string{"admin"},
			want:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := roleAllowed(test.role, test.allowed...); got != test.want {
				t.Fatalf("roleAllowed(%q, %v) = %t, want %t", test.role, test.allowed, got, test.want)
			}
		})
	}
}

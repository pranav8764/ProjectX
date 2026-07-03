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

package core

import "testing"

func TestVersionConstraintCheck(t *testing.T) {
	testCases := []struct {
		name       string
		current    string
		constraint string
		expected   bool
	}{
		{
			name:       "less than stable release",
			current:    "v0.8.1",
			constraint: "<v0.9.0",
			expected:   true,
		},
		{
			name:       "same-base prerelease before stable",
			current:    "v0.9.0-rc.1",
			constraint: "<v0.9.0",
			expected:   true,
		},
		{
			name:       "greater than or equal to prerelease",
			current:    "v0.9.0",
			constraint: ">=v0.9.0-rc.1",
			expected:   true,
		},
		{
			name:       "combined constraint in range",
			current:    "v0.9.1",
			constraint: ">=v0.9.0 <v0.10.0",
			expected:   true,
		},
		{
			name:       "combined constraint excludes stable before prerelease floor",
			current:    "v0.9.0-rc.1",
			constraint: ">=v0.9.0 <v0.10.0",
			expected:   false,
		},
		{
			name:       "equal comparator",
			current:    "v0.9.0-rc.6",
			constraint: "=v0.9.0-rc.6",
			expected:   true,
		},
		{
			name:       "less than or equal comparator",
			current:    "v0.9.0-rc.6",
			constraint: "<=v0.9.0-rc.6",
			expected:   true,
		},
		{
			name:       "greater than comparator excludes equal version",
			current:    "v0.9.0",
			constraint: ">v0.9.0",
			expected:   false,
		},
		{
			name:       "greater than comparator matches next major release",
			current:    "v1.0.0",
			constraint: ">v0.9.0",
			expected:   true,
		},
		{
			name:       "greater than comparator includes later patch in same minor line",
			current:    "v0.9.1",
			constraint: ">v0.9.0",
			expected:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := VersionConstraintCheck(tc.current, tc.constraint)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if got != tc.expected {
				t.Fatalf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestVersionConstraintCheckRejectsInvalidInput(t *testing.T) {
	if _, err := VersionConstraintCheck("0.9.0", "<v0.9.0"); err == nil {
		t.Fatal("expected invalid current version error")
	}

	if _, err := VersionConstraintCheck("v0.9.0", "<0.9.0"); err == nil {
		t.Fatal("expected invalid constraint version error")
	}

	if _, err := VersionConstraintCheck("v0.9.0", "v1.0.0"); err == nil {
		t.Fatal("expected invalid comparator error")
	}

	if _, err := VersionConstraintCheck("v0.9.0", ""); err == nil {
		t.Fatal("expected empty constraint error")
	}
}

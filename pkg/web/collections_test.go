package web

import (
	"strings"
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

var testSources = map[string]dogeboxd.ManifestSourceList{
	"dogeorg.pups": {
		Config: dogeboxd.ManifestSourceConfiguration{ID: "dogeorg.pups"},
		Pups: []dogeboxd.ManifestSourcePup{
			{Name: "Dogecoin Core", Version: "1.2.3"},
			{Name: "Dogecoin Core", Version: "1.2.9"},
			{Name: "Dogecoin Core", Version: "1.3.0"},
			{Name: "Dogecoin Core", Version: "2.0.0"},
			{Name: "Dogenet", Version: "0.0.2"},
		},
	},
}

func TestResolveVersionConstraint(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		pupName    string
		want       string
	}{
		{"exact version", "1.2.3", "Dogecoin Core", "1.2.3"},
		{"latest picks highest", "latest", "Dogecoin Core", "2.0.0"},
		{"tilde allows patch bumps", "~1.2.3", "Dogecoin Core", "1.2.9"},
		{"caret allows minor bumps", "^1.2.3", "Dogecoin Core", "1.3.0"},
		{"latest for single-version pup", "latest", "Dogenet", "0.0.2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveVersionConstraint(testSources, "dogeorg.pups", tt.pupName, tt.constraint)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestResolveVersionConstraint_NoMatch(t *testing.T) {
	_, err := resolveVersionConstraint(testSources, "dogeorg.pups", "Dogecoin Core", "~0.2.0")
	if err == nil {
		t.Fatal("expected error for no matching version")
	}
	if !strings.Contains(err.Error(), "no matching version") {
		t.Fatalf("expected 'no matching version' in error, got: %v", err)
	}
}

func TestResolveVersionConstraint_SourceNotFound(t *testing.T) {
	_, err := resolveVersionConstraint(testSources, "nonexistent", "Dogecoin Core", "latest")
	if err == nil {
		t.Fatal("expected error for missing source")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' in error, got: %v", err)
	}
}

func TestResolveVersionConstraint_UnsupportedFormat(t *testing.T) {
	for _, constraint := range []string{">=0.1.0", "1.x", "*", ""} {
		t.Run(constraint, func(t *testing.T) {
			_, err := resolveVersionConstraint(testSources, "dogeorg.pups", "Dogecoin Core", constraint)
			if err == nil {
				t.Fatalf("expected error for constraint %q", constraint)
			}
		})
	}
}

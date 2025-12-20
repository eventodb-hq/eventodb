package eventodb

import (
	"context"
	"regexp"
	"testing"
)

func TestSYS001_GetServerVersion(t *testing.T) {
	tc := setupTest(t, "sys-001")
	ctx := context.Background()

	version, err := tc.client.SystemVersion(ctx)
	if err != nil {
		t.Fatalf("Failed to get version: %v", err)
	}

	if version == "" {
		t.Error("Expected non-empty version string")
	}

	// Verify it looks like a semver (e.g., "1.3.0")
	semverPattern := regexp.MustCompile(`^\d+\.\d+\.\d+`)
	if !semverPattern.MatchString(version) {
		t.Logf("Warning: version '%s' doesn't match semver pattern (may be dev version)", version)
	}

	t.Logf("Server version: %s", version)
}

func TestSYS002_GetServerHealth(t *testing.T) {
	tc := setupTest(t, "sys-002")
	ctx := context.Background()

	health, err := tc.client.SystemHealth(ctx)
	if err != nil {
		t.Fatalf("Failed to get health: %v", err)
	}

	if health.Status == "" {
		t.Error("Expected status to be set")
	}

	// Status should be "ok" or similar
	if health.Status != "ok" && health.Status != "healthy" {
		t.Logf("Warning: unexpected health status: %s", health.Status)
	}

	t.Logf("Server health: %s", health.Status)
}

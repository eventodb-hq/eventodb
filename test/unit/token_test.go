package unit

import (
	"strings"
	"testing"

	"github.com/message-db/message-db/internal/auth"
)

// Test MDB002_2A_T1: Test token generation creates valid format
func TestMDB002_2A_T1_TokenGenerationCreatesValidFormat(t *testing.T) {
	namespace := "tenant-a"

	token, err := auth.GenerateToken(namespace)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Verify token format: ns_<base64>_<64-hex-chars>
	if !strings.HasPrefix(token, "ns_") {
		t.Errorf("Token should start with 'ns_', got: %s", token)
	}

	parts := strings.Split(strings.TrimPrefix(token, "ns_"), "_")
	if len(parts) != 2 {
		t.Errorf("Token should have 2 parts after 'ns_', got %d parts", len(parts))
	}

	// Verify random part is 64 hex characters
	randomPart := parts[1]
	if len(randomPart) != 64 {
		t.Errorf("Random part should be 64 hex chars, got %d", len(randomPart))
	}

	// Verify it's valid hex
	for _, c := range randomPart {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("Random part contains non-hex character: %c", c)
		}
	}
}

// Test MDB002_2A_T2: Test token parsing extracts correct namespace
func TestMDB002_2A_T2_TokenParsingExtractsNamespace(t *testing.T) {
	originalNamespace := "tenant-a"

	token, err := auth.GenerateToken(originalNamespace)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	parsedNamespace, err := auth.ParseToken(token)
	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}

	if parsedNamespace != originalNamespace {
		t.Errorf("Expected namespace '%s', got '%s'", originalNamespace, parsedNamespace)
	}
}

// Test MDB002_2A_T3: Test token hash is deterministic
func TestMDB002_2A_T3_TokenHashIsDeterministic(t *testing.T) {
	token := "ns_dGVzdA_1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	hash1 := auth.HashToken(token)
	hash2 := auth.HashToken(token)

	if hash1 != hash2 {
		t.Errorf("Token hash should be deterministic, got different values: %s vs %s", hash1, hash2)
	}

	// Verify hash is 64 hex characters (SHA-256)
	if len(hash1) != 64 {
		t.Errorf("Hash should be 64 hex characters (SHA-256), got %d", len(hash1))
	}
}

// Test MDB002_2A_T4: Test invalid token format returns error
func TestMDB002_2A_T4_InvalidTokenFormatReturnsError(t *testing.T) {
	testCases := []struct {
		name  string
		token string
	}{
		{"Missing prefix", "dGVzdA_1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"},
		{"Wrong prefix", "tk_dGVzdA_1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"},
		{"Missing underscore", "ns_dGVzdA1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"},
		{"Too few parts", "ns_dGVzdA"},
		{"Too many parts", "ns_dGVzdA_abc_def"},
		{"Short random part", "ns_dGVzdA_123"},
		{"Long random part", "ns_dGVzdA_1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef123"},
		{"Invalid hex", "ns_dGVzdA_ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ"},
		{"Empty namespace", "ns__1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := auth.ParseToken(tc.token)
			if err == nil {
				t.Errorf("Expected error for invalid token format: %s", tc.name)
			}
		})
	}
}

// Test that empty namespace is rejected
func TestMDB002_2A_EmptyNamespaceRejected(t *testing.T) {
	_, err := auth.GenerateToken("")
	if err == nil {
		t.Error("Expected error when generating token with empty namespace")
	}
}

// Test that different tokens have different random parts
func TestMDB002_2A_DifferentTokensHaveDifferentRandomParts(t *testing.T) {
	token1, _ := auth.GenerateToken("test")
	token2, _ := auth.GenerateToken("test")

	if token1 == token2 {
		t.Error("Different token generations should produce different random parts")
	}

	// Both should parse to same namespace
	ns1, _ := auth.ParseToken(token1)
	ns2, _ := auth.ParseToken(token2)

	if ns1 != ns2 || ns1 != "test" {
		t.Errorf("Both tokens should parse to 'test' namespace, got %s and %s", ns1, ns2)
	}
}

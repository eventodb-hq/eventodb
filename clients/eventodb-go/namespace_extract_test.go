package eventodb

import (
	"encoding/base64"
	"testing"
)

func TestExtractNamespace(t *testing.T) {
	tests := []struct {
		name          string
		token         string
		wantNamespace string
		wantErr       bool
	}{
		{
			name:          "default namespace",
			token:         "ns_" + base64.RawURLEncoding.EncodeToString([]byte("default")) + "_0000000000000000",
			wantNamespace: "default",
			wantErr:       false,
		},
		{
			name:          "custom namespace",
			token:         "ns_" + base64.RawURLEncoding.EncodeToString([]byte("my-app-namespace")) + "_abc123",
			wantNamespace: "my-app-namespace",
			wantErr:       false,
		},
		{
			name:          "namespace with special chars",
			token:         "ns_" + base64.RawURLEncoding.EncodeToString([]byte("tenant/123")) + "_xyz",
			wantNamespace: "tenant/123",
			wantErr:       false,
		},
		{
			name:    "invalid format - missing prefix",
			token:   "invalid_token",
			wantErr: true,
		},
		{
			name:    "invalid format - wrong prefix",
			token:   "tk_abc_def",
			wantErr: true,
		},
		{
			name:    "invalid format - too few parts",
			token:   "ns_abc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractNamespace(tt.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractNamespace() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantNamespace {
				t.Errorf("ExtractNamespace() = %v, want %v", got, tt.wantNamespace)
			}
		})
	}
}

func TestClientGetNamespace(t *testing.T) {
	// Create client with token
	token := "ns_" + base64.RawURLEncoding.EncodeToString([]byte("test-ns")) + "_signature"
	client := NewClient("http://localhost:8080", WithToken(token))

	if got := client.GetNamespace(); got != "test-ns" {
		t.Errorf("GetNamespace() = %v, want %v", got, "test-ns")
	}
}

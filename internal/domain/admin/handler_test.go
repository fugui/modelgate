package admin

import (
	"strings"
	"testing"
)

func TestValidateResourceID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"Valid simple ID", "gpt4", false},
		{"Valid ID with hyphens", "gpt-4-turbo", false},
		{"Valid ID with underscores", "model_v1_0", false},
		{"Valid ID with dots", "gpt-4.5", false},
		{"Valid ID with dots and hyphens", "claude-3.5-sonnet", false},
		{"Valid backend ID with dots", "backend.1.us-east", false},
		{"Valid complex ID", "qwen.2.5-72b_instruct-v1.0", false},

		{"Invalid empty ID", "", true},
		{"Invalid leading dot", ".gpt-4", true},
		{"Invalid leading hyphen", "-gpt-4", true},
		{"Invalid leading underscore", "_gpt-4", true},
		{"Invalid slash", "meta-llama/Llama-2", true},
		{"Invalid space", "gpt 4", true},
		{"Invalid special character @", "model@1", true},
		{"Invalid special character ?", "model?name", true},
		{"Invalid too long ID", strings.Repeat("a", 129), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResourceID(tt.id, "测试ID")
			if (err != nil) != tt.wantErr {
				t.Errorf("validateResourceID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

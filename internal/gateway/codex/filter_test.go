package codex

import (
	"testing"
)

func TestCleanCodeFilterNonStream(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Normal code without fences",
			input:    "    return a + b",
			expected: "    return a + b",
		},
		{
			name:     "Code with leading and trailing markdown fences",
			input:    "```python\ndef add(a, b):\n    return a + b\n```",
			expected: "def add(a, b):\n    return a + b",
		},
		{
			name:     "Code with tilde fences",
			input:    "~~~go\nfunc main() {}\n~~~",
			expected: "func main() {}",
		},
		{
			name:     "Code with whitespace before fence",
			input:    "   ```javascript\nconsole.log('hi');\n```  ",
			expected: "console.log('hi');",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CleanCodeFilterNonStream(tt.input)
			if got != tt.expected {
				t.Errorf("CleanCodeFilterNonStream(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCleanCodeFilterStream(t *testing.T) {
	state := make(map[string]interface{})

	// Chunk 1: ```python\n
	chunk1 := "```python\n"
	out1 := CleanCodeFilterStream(chunk1, state)
	if out1 != "" {
		t.Errorf("Expected empty output for chunk1, got %q", out1)
	}

	// Chunk 2: def hello():\n
	chunk2 := "def hello():\n"
	out2 := CleanCodeFilterStream(chunk2, state)
	if out2 != "def hello():\n" {
		t.Errorf("Expected 'def hello():\\n', got %q", out2)
	}
}

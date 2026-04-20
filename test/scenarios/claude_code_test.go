package scenarios

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"modelgate/internal/anthropic"
)

// TestClaudeCodeProtocolConversion validates the Anthropic to OpenAI and back protocol conversion
// using real-world payloads captured from Claude Code.
func TestClaudeCodeProtocolConversion(t *testing.T) {
	testDataDir := "testdata/claude_code"
	entries, err := os.ReadDir(testDataDir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("testdata/claude_code directory not found, skipping Claude Code protocol tests")
		}
		t.Fatalf("Failed to read testdata directory: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		
		scenarioDir := filepath.Join(testDataDir, entry.Name())
		t.Run(entry.Name(), func(t *testing.T) {
			// 1. Load Original Client Request
			clientReqPath := filepath.Join(scenarioDir, "1_client_request.json")
			clientReqData, err := os.ReadFile(clientReqPath)
			if err != nil {
				if os.IsNotExist(err) {
					t.Skip("Missing 1_client_request.json, skipping scenario")
				}
				t.Fatalf("Failed to read client request: %v", err)
			}

			var anthropicReq anthropic.MessagesRequest
			err = json.Unmarshal(clientReqData, &anthropicReq)
			require.NoError(t, err, "Failed to parse Anthropic request")

			// 2. Test Request Conversion to OpenAI
			openaiBody, err := anthropic.ConvertToOpenAI(&anthropicReq)
			require.NoError(t, err, "ConvertToOpenAI should succeed")

			var openaiReq map[string]interface{}
			err = json.Unmarshal(openaiBody, &openaiReq)
			require.NoError(t, err, "Converted OpenAI body should be valid JSON")

			// Check tool calls exist in OpenAI request if they existed in Anthropic
			if len(anthropicReq.Tools) > 0 {
				tools, ok := openaiReq["tools"].([]interface{})
				assert.True(t, ok && len(tools) > 0, "OpenAI request should have tools")
			}

			// 3. Test Response Stream Conversion
			backendRespPath := filepath.Join(scenarioDir, "3_backend_response.txt")
			backendRespData, err := os.ReadFile(backendRespPath)
			if err != nil {
				if os.IsNotExist(err) {
					// No backend response to test, just test the request
					return
				}
				t.Fatalf("Failed to read backend response: %v", err)
			}

			// Simulate stream processing
			state := make(map[string]interface{})
			lines := strings.Split(string(backendRespData), "\n")
			var convertedLines []string

			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}

				if strings.HasPrefix(line, ":") {
					convertedLines = append(convertedLines, line)
					continue
				}

				// The stream line processing
				convertedLine, err := anthropic.ConvertStreamLine(line, &anthropicReq, state)
				require.NoError(t, err, "ConvertStreamLine should not fail on valid stream line")
				
				if convertedLine != "" && convertedLine != line {
					// Extract event parts
					parts := strings.Split(convertedLine, "\n\n")
					for _, part := range parts {
						if strings.TrimSpace(part) != "" {
							convertedLines = append(convertedLines, strings.TrimSpace(part))
						}
					}
				} else if convertedLine != "" {
					convertedLines = append(convertedLines, convertedLine)
				}
			}

			// Verify if it has successfully generated Anthropic tool use blocks
			foundToolUse := false
			for _, cl := range convertedLines {
				if strings.Contains(cl, `"type":"tool_use"`) {
					foundToolUse = true
					// Check tool ID prefix
					assert.Contains(t, cl, `"toolu_`, "Tool ID should be prefixed with toolu_")
				}
				if strings.Contains(cl, `"stop_reason":"tool_use"`) {
					foundToolUse = true
				}
			}

			if strings.Contains(string(backendRespData), `"tool_calls"`) {
				assert.True(t, foundToolUse, "Expected to find tool_use conversion when backend returned tool_calls")
			}
		})
	}
}

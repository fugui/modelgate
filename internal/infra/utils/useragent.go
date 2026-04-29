package utils

import (
	"regexp"
	"strings"
)

// ParseClientType 将 User-Agent 字符串解析为简短的客户端类型标识
func ParseClientType(userAgent string) string {
	if userAgent == "" {
		return "Unknown"
	}

	ua := strings.ToLower(userAgent)

	// IDE / 开发工具（优先匹配，因为它们的 UA 可能包含浏览器关键字）
	switch {
	case strings.Contains(ua, "claude-code") || strings.Contains(ua, "claude-cli"):
		return "Claude Code"
	case strings.Contains(ua, "opencode"):
		return "OpenCode"
	case strings.Contains(ua, "cursor"):
		return "Cursor"
	case strings.Contains(ua, "vscode") || strings.Contains(ua, "visual studio code"):
		return "VS Code"
	case strings.Contains(ua, "jetbrains") || strings.Contains(ua, "intellij"):
		return "JetBrains"
	case strings.Contains(ua, "copilot"):
		return "Copilot"
	}

	// 编程语言 HTTP 客户端
	switch {
	case strings.Contains(ua, "python-requests") || strings.Contains(ua, "httpx") || strings.Contains(ua, "aiohttp"):
		return "Python"
	case strings.Contains(ua, "go-http-client") || strings.Contains(ua, "go-resty"):
		return "Go"
	case strings.Contains(ua, "axios") || strings.Contains(ua, "node-fetch") || strings.Contains(ua, "undici"):
		return "Node.js"
	case strings.Contains(ua, "curl"):
		return "curl"
	case strings.Contains(ua, "wget"):
		return "wget"
	case strings.Contains(ua, "postman"):
		return "Postman"
	}

	// 浏览器（注意顺序：Edge 的 UA 包含 Chrome，Safari 的 UA 也包含 Safari）
	switch {
	case strings.Contains(ua, "edg/"):
		return "Edge"
	case strings.Contains(ua, "chrome/") && !strings.Contains(ua, "edg/"):
		return "Chrome"
	case strings.Contains(ua, "firefox/"):
		return "Firefox"
	case strings.Contains(ua, "safari/") && !strings.Contains(ua, "chrome/"):
		return "Safari"
	case strings.Contains(ua, "mozilla/"):
		return "Browser"
	}

	return "Unknown"
}

// FormatUserAgentForDisplay 将原始 UserAgent 和 Referer 解析为前端友好的显示名称
func FormatUserAgentForDisplay(userAgent string, referer string) string {
	// 如果来源是网页的 /chat，认为是演练场
	if referer != "" && strings.Contains(referer, "/chat") {
		return "演练场"
	}

	if userAgent == "" {
		return "Unknown"
	}

	ua := userAgent
	uaLower := strings.ToLower(ua)

	// 提取名称和版本号，例如 "Claude Code/2.1.84" 或 "Cursor 0.45.0"
	re := regexp.MustCompile(`(?i)(claude-code|claude-cli|opencode|cursor|vscode|jetbrains|intellij|copilot|postman|curl|wget)[/\s]?([0-9a-zA-Z.-]+)?`)
	matches := re.FindStringSubmatch(ua)

	if len(matches) > 1 {
		name := matches[1]
		version := ""
		if len(matches) > 2 {
			version = matches[2]
		}

		// 规范化名称
		switch strings.ToLower(name) {
		case "claude-code", "claude-cli":
			name = "Claude Code"
		case "opencode":
			name = "OpenCode"
		case "cursor":
			name = "Cursor"
		case "vscode":
			name = "VS Code"
		case "jetbrains", "intellij":
			name = "JetBrains"
		case "postman":
			name = "Postman"
		case "curl":
			name = "curl"
		case "wget":
			name = "wget"
		}

		if version != "" {
			return name + " " + version
		}
		return name
	}

	// 编程语言 HTTP 客户端
	if strings.Contains(uaLower, "python-requests") || strings.Contains(uaLower, "httpx") || strings.Contains(uaLower, "aiohttp") {
		return "Python"
	}
	if strings.Contains(uaLower, "go-http-client") || strings.Contains(uaLower, "go-resty") {
		return "Go"
	}
	if strings.Contains(uaLower, "axios") || strings.Contains(uaLower, "node-fetch") || strings.Contains(uaLower, "undici") {
		return "Node.js"
	}

	// 浏览器
	if strings.Contains(uaLower, "edg/") {
		re := regexp.MustCompile(`(?i)edg/([0-9a-zA-Z.-]+)`)
		if m := re.FindStringSubmatch(ua); len(m) > 1 {
			return "Edge " + m[1]
		}
		return "Edge"
	}
	if strings.Contains(uaLower, "chrome/") && !strings.Contains(uaLower, "edg/") {
		re := regexp.MustCompile(`(?i)chrome/([0-9a-zA-Z.-]+)`)
		if m := re.FindStringSubmatch(ua); len(m) > 1 {
			return "Chrome " + m[1]
		}
		return "Chrome"
	}
	if strings.Contains(uaLower, "firefox/") {
		re := regexp.MustCompile(`(?i)firefox/([0-9a-zA-Z.-]+)`)
		if m := re.FindStringSubmatch(ua); len(m) > 1 {
			return "Firefox " + m[1]
		}
		return "Firefox"
	}
	if strings.Contains(uaLower, "safari/") && !strings.Contains(uaLower, "chrome/") {
		re := regexp.MustCompile(`(?i)version/([0-9a-zA-Z.-]+)`)
		if m := re.FindStringSubmatch(ua); len(m) > 1 {
			return "Safari " + m[1]
		}
		return "Safari"
	}
	if strings.Contains(uaLower, "mozilla/") {
		return "Browser"
	}

	// 如果没有匹配上，则返回截断的原始 UA 以避免过长
	if len(ua) > 30 {
		return ua[:27] + "..."
	}
	return ua
}

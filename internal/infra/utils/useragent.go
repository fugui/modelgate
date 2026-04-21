package utils

import "strings"

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

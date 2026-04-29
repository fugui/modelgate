package utils

import (
	"regexp"
	"strings"
)

// clientRule 客户端匹配规则
type clientRule struct {
	keywords []string          // UA 中需要包含的关键字（任一匹配即可）
	exclude  []string          // UA 中不能包含的关键字
	name     string            // 规范化名称
	versionRe *regexp.Regexp   // 版本号提取正则（nil 表示不提取版本）
}

// clientRules 按优先级排列的客户端识别规则
// IDE/开发工具优先于浏览器（因为 IDE 的 UA 可能包含浏览器关键字）
var clientRules = []clientRule{
	// IDE / 开发工具
	{keywords: []string{"claude-code", "claude-cli"}, name: "Claude Code",
		versionRe: regexp.MustCompile(`(?i)(?:claude-code|claude-cli)[/\s]?([0-9a-zA-Z.-]+)`)},
	{keywords: []string{"opencode"}, name: "OpenCode",
		versionRe: regexp.MustCompile(`(?i)opencode[/\s]?([0-9a-zA-Z.-]+)`)},
	{keywords: []string{"cursor"}, name: "Cursor",
		versionRe: regexp.MustCompile(`(?i)cursor[/\s]?([0-9a-zA-Z.-]+)`)},
	{keywords: []string{"vscode", "visual studio code"}, name: "VS Code",
		versionRe: regexp.MustCompile(`(?i)vscode[/\s]?([0-9a-zA-Z.-]+)`)},
	{keywords: []string{"jetbrains", "intellij"}, name: "JetBrains",
		versionRe: regexp.MustCompile(`(?i)(?:jetbrains|intellij)[/\s]?([0-9a-zA-Z.-]+)`)},
	{keywords: []string{"copilot"}, name: "Copilot"},

	// 编程语言 HTTP 客户端
	{keywords: []string{"python-requests", "httpx", "aiohttp"}, name: "Python"},
	{keywords: []string{"go-http-client", "go-resty"}, name: "Go"},
	{keywords: []string{"axios", "node-fetch", "undici"}, name: "Node.js"},

	// 命令行工具
	{keywords: []string{"curl"}, name: "curl",
		versionRe: regexp.MustCompile(`(?i)curl[/\s]?([0-9a-zA-Z.-]+)`)},
	{keywords: []string{"wget"}, name: "wget",
		versionRe: regexp.MustCompile(`(?i)wget[/\s]?([0-9a-zA-Z.-]+)`)},
	{keywords: []string{"postman"}, name: "Postman",
		versionRe: regexp.MustCompile(`(?i)postman[/\s]?([0-9a-zA-Z.-]+)`)},

	// 浏览器（注意顺序：Edge 的 UA 包含 Chrome，Safari 的 UA 也包含 Safari）
	{keywords: []string{"edg/"}, name: "Edge",
		versionRe: regexp.MustCompile(`(?i)edg/([0-9a-zA-Z.-]+)`)},
	{keywords: []string{"chrome/"}, exclude: []string{"edg/"}, name: "Chrome",
		versionRe: regexp.MustCompile(`(?i)chrome/([0-9a-zA-Z.-]+)`)},
	{keywords: []string{"firefox/"}, name: "Firefox",
		versionRe: regexp.MustCompile(`(?i)firefox/([0-9a-zA-Z.-]+)`)},
	{keywords: []string{"safari/"}, exclude: []string{"chrome/"}, name: "Safari",
		versionRe: regexp.MustCompile(`(?i)version/([0-9a-zA-Z.-]+)`)},
	{keywords: []string{"mozilla/"}, name: "Browser"},
}

// parseUA 解析 User-Agent 字符串，返回客户端名称和版本号
func parseUA(userAgent string) (name, version string) {
	if userAgent == "" {
		return "Unknown", ""
	}

	uaLower := strings.ToLower(userAgent)

	for _, rule := range clientRules {
		matched := false
		for _, kw := range rule.keywords {
			if strings.Contains(uaLower, kw) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}

		// 检查排除关键字
		excluded := false
		for _, ex := range rule.exclude {
			if strings.Contains(uaLower, ex) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		// 提取版本号
		ver := ""
		if rule.versionRe != nil {
			if m := rule.versionRe.FindStringSubmatch(userAgent); len(m) > 1 {
				ver = m[1]
			}
		}

		return rule.name, ver
	}

	// 未匹配到任何规则
	return "", ""
}

// ParseClientType 将 User-Agent 字符串解析为简短的客户端类型标识（不含版本号）
func ParseClientType(userAgent string) string {
	name, _ := parseUA(userAgent)
	if name == "" {
		return "Unknown"
	}
	return name
}

// FormatUserAgentForDisplay 将原始 UserAgent 和 Referer 解析为前端友好的显示名称（含版本号）
func FormatUserAgentForDisplay(userAgent string, referer string) string {
	// 如果来源是网页的 /chat，认为是演练场
	if referer != "" && strings.Contains(referer, "/chat") {
		return "演练场"
	}

	name, version := parseUA(userAgent)
	if name == "" {
		// 未匹配：返回截断的原始 UA
		if len(userAgent) > 30 {
			return userAgent[:27] + "..."
		}
		if userAgent == "" {
			return "Unknown"
		}
		return userAgent
	}

	if version != "" {
		return name + " " + version
	}
	return name
}

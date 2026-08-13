package codex

import (
	"regexp"
	"strings"
)

var (
	leadingFenceRe  = regexp.MustCompile(`^(?s)^\s*(` + "```" + `|~~~)[a-zA-Z0-9_-]*\r?\n?`)
	trailingFenceRe = regexp.MustCompile(`(?s)\r?\n?\s*(` + "```" + `|~~~)\s*$`)
)

// CleanCodeFilterNonStream 过滤非流式代码补全中的 Markdown Codeblock 标记
func CleanCodeFilterNonStream(text string) string {
	if text == "" {
		return ""
	}
	// 剥离首部 ```lang\n 或 ~~~lang\n
	text = leadingFenceRe.ReplaceAllString(text, "")
	// 剥离尾部 \n``` 或 \n~~~
	text = trailingFenceRe.ReplaceAllString(text, "")
	return text
}

// CleanCodeFilterStream 处理流式文本中的 Markdown 围栏剥离
// 使用 state 保存跨 chunk 的缓冲状态
func CleanCodeFilterStream(delta string, state map[string]interface{}) string {
	if delta == "" {
		return ""
	}

	// 标记是否已处理过首部围栏
	firstChunkHandled, _ := state["first_fence_handled"].(bool)
	if !firstChunkHandled {
		// 读取历史前导缓存
		headBuf, _ := state["head_fence_buf"].(string)
		combined := headBuf + delta

		trimmed := strings.TrimLeft(combined, " \t")

		// 完整围栏开头（``` 或 ~~~）
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			// 匹配首部围栏并剥离
			cleaned := leadingFenceRe.ReplaceAllString(trimmed, "")
			if len(cleaned) < len(trimmed) {
				// 成功剥离了首部围栏
				state["first_fence_handled"] = true
				state["head_fence_buf"] = ""
				return cleaned
			}
			// 还在围栏定义行中，尚未读到换行，暂存
			if !strings.Contains(combined, "\n") && len(combined) < 40 {
				state["head_fence_buf"] = combined
				return ""
			}
			state["first_fence_handled"] = true
			state["head_fence_buf"] = ""
			return combined
		}

		// 跨 chunk 分片的围栏前缀（"`" / "``" / "~" / "~~"），继续缓冲等待补全
		if !strings.Contains(combined, "\n") && len(combined) <= 2 &&
			(strings.HasPrefix(combined, "`") || strings.HasPrefix(combined, "~")) {
			state["head_fence_buf"] = combined
			return ""
		}

		// 不符合首部围栏特征
		state["first_fence_handled"] = true
		state["head_fence_buf"] = ""
		return combined
	}

	// 尾部围栏由 stream 处理器通过 tail_buf 缓冲 + TrimTrailingFence 在收尾时处理
	return delta
}

// TrimTrailingFence 剥离结尾残留的围栏
func TrimTrailingFence(fullText string) string {
	return trailingFenceRe.ReplaceAllString(fullText, "")
}

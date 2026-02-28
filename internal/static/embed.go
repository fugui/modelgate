package static

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed dist
var embeddedFS embed.FS

// FS 返回嵌入的文件系统（不包含 dist 前缀）
func FS() (fs.FS, error) {
	return fs.Sub(embeddedFS, "dist")
}

// Serve 返回一个处理静态文件的 gin HandlerFunc
func Serve() gin.HandlerFunc {
	staticFS, err := FS()
	if err != nil {
		// 如果嵌入失败，返回空处理函数
		return func(c *gin.Context) {
			c.Next()
		}
	}

	fileServer := http.FileServer(http.FS(staticFS))

	return func(c *gin.Context) {
		// 只处理非 API 请求
		if strings.HasPrefix(c.Request.URL.Path, "/api/") ||
			strings.HasPrefix(c.Request.URL.Path, "/v1/") {
			c.Next()
			return
		}

		// 尝试提供静态文件
		filepath := path.Clean(c.Request.URL.Path)
		if filepath == "/" {
			filepath = "/index.html"
		}

		// 检查文件是否存在
		file, err := staticFS.Open(strings.TrimPrefix(filepath, "/"))
		if err != nil {
			// 文件不存在，返回 index.html（支持前端路由）
			// 禁用缓存，确保前端路由能正确处理鉴权
			c.Writer.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Writer.Header().Set("Pragma", "no-cache")
			c.Writer.Header().Set("Expires", "0")
			c.Request.URL.Path = "/"
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}
		file.Close()

		// 文件存在，直接提供
		// 对 index.html 禁用缓存，确保前端路由能正确处理鉴权
		if filepath == "/index.html" {
			c.Writer.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Writer.Header().Set("Pragma", "no-cache")
			c.Writer.Header().Set("Expires", "0")
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}
}

.PHONY: build run test clean lint fmt deps web-build embed release

# 变量
BINARY_NAME=modelgate
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.Commit=$(COMMIT)"

# 默认构建（包含前端嵌入）
build: web-build embed
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/server
	@echo "构建完成: $(BINARY_NAME)"

# 仅构建 Go（不构建前端，用于开发）
build-go:
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/server

# 构建前端
web-build:
	@echo "构建前端..."
	cd web && npm install && npm run build

# 复制前端文件到嵌入目录
embed: web-build
	@echo "准备嵌入文件..."
	@rm -rf internal/static/dist
	@mkdir -p internal/static/dist
	@cp -r web/dist/* internal/static/dist/
	@echo "已复制 $$(ls internal/static/dist | wc -l) 个文件"

# 开发模式运行（使用 vite 代理）
dev:
	@echo "启动开发服务器..."
	@echo "前端: http://localhost:5173"
	@echo "后端: http://localhost:8080"
	@make -j2 dev-web dev-server

dev-web:
	cd web && npm run dev

dev-server:
	go run ./cmd/server

# 运行（生产模式，需要先 build）
run: build
	./$(BINARY_NAME)

# 运行测试
test:
	go test -v ./...

# 清理
clean:
	rm -f $(BINARY_NAME)
	rm -rf internal/static/dist
	go clean

# 代码检查
lint:
	golangci-lint run

# 格式化代码
fmt:
	go fmt ./...
	cd web && npm run lint -- --fix 2>/dev/null || true

# 安装依赖
deps:
	go mod download
	go mod tidy
	cd web && npm install

# 跨平台构建
release: clean web-build embed
	@echo "构建多平台发布版本..."
	@mkdir -p releases

	# Linux AMD64
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o releases/$(BINARY_NAME)-linux-amd64 ./cmd/server
	cp config.yaml releases/config.yaml.example
	cp README.md releases/
	cd releases && tar -czf $(BINARY_NAME)-$(VERSION)-linux-amd64.tar.gz $(BINARY_NAME)-linux-amd64 config.yaml.example README.md

	# Linux ARM64
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o releases/$(BINARY_NAME)-linux-arm64 ./cmd/server
	cd releases && tar -czf $(BINARY_NAME)-$(VERSION)-linux-arm64.tar.gz $(BINARY_NAME)-linux-arm64 config.yaml.example README.md

	# Darwin AMD64
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o releases/$(BINARY_NAME)-darwin-amd64 ./cmd/server
	cd releases && tar -czf $(BINARY_NAME)-$(VERSION)-darwin-amd64.tar.gz $(BINARY_NAME)-darwin-amd64 config.yaml.example README.md

	# Darwin ARM64 (M1/M2)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o releases/$(BINARY_NAME)-darwin-arm64 ./cmd/server
	cd releases && tar -czf $(BINARY_NAME)-$(VERSION)-darwin-arm64.tar.gz $(BINARY_NAME)-darwin-arm64 config.yaml.example README.md

	# Windows AMD64
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o releases/$(BINARY_NAME)-windows-amd64.exe ./cmd/server
	cd releases && zip -r $(BINARY_NAME)-$(VERSION)-windows-amd64.zip $(BINARY_NAME)-windows-amd64.exe config.yaml.example README.md

	@echo ""
	@echo "发布包已创建:"
	@ls -lh releases/*.tar.gz releases/*.zip 2>/dev/null

# 快速启动（使用现有构建）
start:
	@if [ ! -f $(BINARY_NAME) ]; then \
		echo "未找到 $(BINARY_NAME)，先执行构建..."; \
		make build; \
	fi
	./$(BINARY_NAME)

# 查看版本信息
version:
	@echo "Version: $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Commit: $(COMMIT)"

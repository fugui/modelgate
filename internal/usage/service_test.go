package usage

import (
	"container/ring"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestRecordAccessAndGetRecentAccess(t *testing.T) {
	// 创建服务实例（不需要真实的 logger）
	service := &Service{
		accessLogs: make(map[uuid.UUID]*ring.Ring),
		maxLogs:    20,
	}

	userID := uuid.New()

	// 记录几条日志
	service.RecordAccess(userID, "GET", "/api/v1/user/quota", "192.168.1.1", "TestAgent/1.0", 200, 100, 200, 50)
	service.RecordAccess(userID, "POST", "/api/v1/chat/completions", "192.168.1.1", "TestAgent/1.0", 200, 1000, 5000, 120)
	service.RecordAccess(userID, "GET", "/api/v1/models", "192.168.1.1", "TestAgent/1.0", 200, 50, 1000, 30)

	// 获取访问日志
	logs := service.GetRecentAccess(userID, 20)

	// 验证结果
	assert.Equal(t, 3, len(logs), "应该有3条访问日志")

	// 验证倒序排列（最新的在前）
	assert.Equal(t, "GET", logs[0].Method, "第一条应该是最新的 GET 请求")
	assert.Equal(t, "POST", logs[1].Method, "第二条应该是 POST 请求")

	// 验证字段
	assert.Equal(t, "/api/v1/models", logs[0].Path)
	assert.Equal(t, 200, logs[0].StatusCode)
	assert.Equal(t, int64(50), logs[0].RequestBytes)
	assert.Equal(t, int64(1000), logs[0].ResponseBytes)
}

func TestGetRecentAccessLimit(t *testing.T) {
	service := &Service{
		accessLogs: make(map[uuid.UUID]*ring.Ring),
		maxLogs:    20,
	}

	userID := uuid.New()

	// 记录25条日志（超过最大值20）
	for i := 0; i < 25; i++ {
		service.RecordAccess(userID, "GET", "/api/test", "127.0.0.1", "Test", 200, 100, 200, 10)
	}

	// 获取所有日志
	logs := service.GetRecentAccess(userID, 20)
	assert.Equal(t, 20, len(logs), "应该只返回20条日志")

	// 测试限制参数
	logs = service.GetRecentAccess(userID, 5)
	assert.Equal(t, 5, len(logs), "应该只返回5条日志")
}

func TestGetRecentAccessEmpty(t *testing.T) {
	service := &Service{
		accessLogs: make(map[uuid.UUID]*ring.Ring),
		maxLogs:    20,
	}

	userID := uuid.New()

	// 获取不存在的用户日志
	logs := service.GetRecentAccess(userID, 20)
	assert.Equal(t, 0, len(logs), "应该返回空数组")
}

func TestConcurrentAccess(t *testing.T) {
	service := &Service{
		accessLogs: make(map[uuid.UUID]*ring.Ring),
		maxLogs:    20,
	}

	userID := uuid.New()
	done := make(chan bool)

	// 并发记录日志
	for i := 0; i < 10; i++ {
		go func() {
			for i := 0; i < 1000; i++ {
				service.RecordAccess(userID, "GET", "/api/test", "127.0.0.1", "Test", 200, 100, 200, 15)
			}
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证日志数量
	logs := service.GetRecentAccess(userID, 20)
	assert.Equal(t, 20, len(logs), "应该只保留最新的20条日志")
}

func TestMultipleUsers(t *testing.T) {
	service := &Service{
		accessLogs: make(map[uuid.UUID]*ring.Ring),
		maxLogs:    20,
	}

	user1 := uuid.New()
	user2 := uuid.New()

	// 为用户1记录日志
	for i := 0; i < 5; i++ {
		service.RecordAccess(user1, "GET", "/api/user1", "127.0.0.1", "Test", 200, 100, 200, 5)
	}
	
	// user2 访问一次
	service.RecordAccess(user2, "POST", "/api/user2", "192.168.1.1", "Test", 200, 200, 300, 8)

	// 验证用户1的日志
	logs1 := service.GetRecentAccess(user1, 20)
	assert.Equal(t, 5, len(logs1))
	assert.Equal(t, "GET", logs1[0].Method)

	// 验证用户2的日志
	logs2 := service.GetRecentAccess(user2, 20)
	assert.Equal(t, 1, len(logs2))
	assert.Equal(t, "POST", logs2[0].Method)
}

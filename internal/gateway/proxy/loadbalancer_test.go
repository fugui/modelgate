package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundRobinBalancer_MaxConcurrencyUnlimited(t *testing.T) {
	lb := NewRoundRobinBalancer()
	backend := Backend{
		ID:             "backend-unlimited",
		URL:            "http://localhost:8001",
		MaxConcurrency: 0, // unlimited
		ModelName:      "gpt-4",
	}
	lb.AddBackend("gpt-4", backend)

	// Acquire concurrency multiple times
	for i := 0; i < 10; i++ {
		ok := lb.AcquireBackend(backend.ID)
		assert.True(t, ok)
	}

	// Verify we can still get the backend (Next also acquires)
	b, model, ok := lb.Next("gpt-4", "")
	require.True(t, ok)
	assert.Equal(t, backend.ID, b.ID)
	assert.Equal(t, "gpt-4", model)
}

func TestRoundRobinBalancer_MaxConcurrencyLimited(t *testing.T) {
	lb := NewRoundRobinBalancer()
	backend := Backend{
		ID:             "backend-limited",
		URL:            "http://localhost:8002",
		MaxConcurrency: 2,
		ModelName:      "gpt-4",
	}
	lb.AddBackend("gpt-4", backend)

	// Acquire twice
	ok := lb.AcquireBackend(backend.ID)
	assert.True(t, ok)
	ok = lb.AcquireBackend(backend.ID)
	assert.True(t, ok)

	// Third acquire should fail
	ok = lb.AcquireBackend(backend.ID)
	assert.False(t, ok)

	// Next() should fail because the only backend is at capacity
	_, _, ok = lb.Next("gpt-4", "")
	assert.False(t, ok)

	// Release one
	lb.ReleaseBackend(backend.ID)

	// Now Next() should succeed (it also acquires)
	b, model, ok := lb.Next("gpt-4", "")
	require.True(t, ok)
	assert.Equal(t, backend.ID, b.ID)
	assert.Equal(t, "gpt-4", model)
}

func TestRoundRobinBalancer_AllUnhealthyFallback(t *testing.T) {
	lb := NewRoundRobinBalancer()
	backend1 := Backend{
		ID:             "backend-1",
		URL:            "http://localhost:8001",
		MaxConcurrency: 0,
		ModelName:      "gpt-4",
	}
	backend2 := Backend{
		ID:             "backend-2",
		URL:            "http://localhost:8002",
		MaxConcurrency: 0,
		ModelName:      "gpt-4",
	}
	lb.AddBackend("gpt-4", backend1)
	lb.AddBackend("gpt-4", backend2)

	// Mark both failed 3 times each (threshold for unhealthy)
	for i := 0; i < 3; i++ {
		lb.MarkFailed(backend1.ID)
		lb.MarkFailed(backend2.ID)
	}

	// Next should still return the first backend as fallback
	b, model, ok := lb.Next("gpt-4", "")
	require.True(t, ok)
	assert.Equal(t, backend1.ID, b.ID)
	assert.Equal(t, "gpt-4", model)
}

func TestRoundRobinBalancer_LeastConnections(t *testing.T) {
	lb := NewRoundRobinBalancer()
	backend1 := Backend{
		ID:             "backend-1",
		URL:            "http://localhost:8001",
		MaxConcurrency: 0,
		ModelName:      "gpt-4",
	}
	backend2 := Backend{
		ID:             "backend-2",
		URL:            "http://localhost:8002",
		MaxConcurrency: 0,
		ModelName:      "gpt-4",
	}
	lb.AddBackend("gpt-4", backend1)
	lb.AddBackend("gpt-4", backend2)

	// Simulate backend-1 having 3 active connections
	lb.AcquireBackend("backend-1")
	lb.AcquireBackend("backend-1")
	lb.AcquireBackend("backend-1")

	// backend-1: 3 connections, backend-2: 0 connections
	// Next() should choose backend-2 (least connections) and acquire it
	b, _, ok := lb.Next("gpt-4", "")
	require.True(t, ok)
	assert.Equal(t, "backend-2", b.ID, "should route to backend with fewer connections")

	// Now backend-1: 3, backend-2: 1 (acquired by Next)
	b, _, ok = lb.Next("gpt-4", "")
	require.True(t, ok)
	assert.Equal(t, "backend-2", b.ID, "should still route to backend with fewer connections")

	// Now backend-1: 3, backend-2: 2
	b, _, ok = lb.Next("gpt-4", "")
	require.True(t, ok)
	assert.Equal(t, "backend-2", b.ID, "should still route to backend with fewer connections")

	// Now backend-1: 3, backend-2: 3 (equal)
	// Release 2 from backend-1
	lb.ReleaseBackend("backend-1")
	lb.ReleaseBackend("backend-1")

	// Now backend-1: 1, backend-2: 3
	b, _, ok = lb.Next("gpt-4", "")
	require.True(t, ok)
	assert.Equal(t, "backend-1", b.ID, "should route to backend with fewer connections after release")
}

func TestRoundRobinBalancer_LeastConnectionsWithMaxConcurrency(t *testing.T) {
	lb := NewRoundRobinBalancer()
	backend1 := Backend{
		ID:             "backend-1",
		URL:            "http://localhost:8001",
		MaxConcurrency: 2,
		ModelName:      "gpt-4",
	}
	backend2 := Backend{
		ID:             "backend-2",
		URL:            "http://localhost:8002",
		MaxConcurrency: 2,
		ModelName:      "gpt-4",
	}
	lb.AddBackend("gpt-4", backend1)
	lb.AddBackend("gpt-4", backend2)

	// Fill backend-1 to capacity
	lb.AcquireBackend("backend-1")
	lb.AcquireBackend("backend-1")

	// backend-1: 2/2 (full), backend-2: 0/2
	b, _, ok := lb.Next("gpt-4", "")
	require.True(t, ok)
	assert.Equal(t, "backend-2", b.ID, "should skip full backend and use available one")

	// backend-1: 2/2, backend-2: 1/2
	b, _, ok = lb.Next("gpt-4", "")
	require.True(t, ok)
	assert.Equal(t, "backend-2", b.ID, "should still use backend-2 since backend-1 is full")

	// backend-1: 2/2, backend-2: 2/2 — both full
	_, _, ok = lb.Next("gpt-4", "")
	assert.False(t, ok, "should return false when all backends are at capacity")

	// Release one from each
	lb.ReleaseBackend("backend-1")
	lb.ReleaseBackend("backend-2")

	// backend-1: 1/2, backend-2: 1/2 — equal, should pick either
	b, _, ok = lb.Next("gpt-4", "")
	require.True(t, ok)
	assert.Contains(t, []string{"backend-1", "backend-2"}, b.ID)
}

// TestRoundRobinBalancer_TieBreaking 验证当后端负载相同时，随机起始索引能打破平局
// 避免流量全部倾斜给切片中的第一个后端
func TestRoundRobinBalancer_TieBreaking(t *testing.T) {
	lb := NewRoundRobinBalancer()
	lb.AddBackend("gpt-4", Backend{ID: "backend-1", URL: "http://localhost:8001", ModelName: "gpt-4"})
	lb.AddBackend("gpt-4", Backend{ID: "backend-2", URL: "http://localhost:8002", ModelName: "gpt-4"})

	counts := map[string]int{"backend-1": 0, "backend-2": 0}
	n := 100
	for i := 0; i < n; i++ {
		b, _, ok := lb.Next("gpt-4", "")
		require.True(t, ok)
		counts[b.ID]++
		lb.ReleaseBackend(b.ID) // 立即释放，保持两个后端并发数均为 0
	}

	// 两个后端都应该获得显著的流量份额（至少 20%）
	assert.Greater(t, counts["backend-1"], n/5,
		"backend-1 should get significant traffic, got %d/%d", counts["backend-1"], n)
	assert.Greater(t, counts["backend-2"], n/5,
		"backend-2 should get significant traffic, got %d/%d", counts["backend-2"], n)
}

// TestRoundRobinBalancer_WeightedLeastConnections 验证加权最少连接策略
// 高权重后端即使绝对并发数更多，但加权负载率更低时应被优先选择
func TestRoundRobinBalancer_WeightedLeastConnections(t *testing.T) {
	lb := NewRoundRobinBalancer()
	lb.AddBackend("gpt-4", Backend{
		ID:        "backend-heavy",
		URL:       "http://localhost:8001",
		Weight:    3,
		ModelName: "gpt-4",
	})
	lb.AddBackend("gpt-4", Backend{
		ID:        "backend-light",
		URL:       "http://localhost:8002",
		Weight:    1,
		ModelName: "gpt-4",
	})

	// backend-heavy: 并发=2, 权重=3 → 加权负载率 = 2/3 ≈ 0.67
	// backend-light: 并发=1, 权重=1 → 加权负载率 = 1/1 = 1.0
	// 应优先选择 backend-heavy（加权负载率更低）
	lb.AcquireBackend("backend-heavy")
	lb.AcquireBackend("backend-heavy")
	lb.AcquireBackend("backend-light")

	b, _, ok := lb.Next("gpt-4", "")
	require.True(t, ok)
	assert.Equal(t, "backend-heavy", b.ID,
		"should prefer backend with lower weighted load (2/3 < 1/1)")
}

// TestRoundRobinBalancer_WeightedDistribution 验证权重比例在负载均衡中的效果
// 高权重后端应该承担更多流量
func TestRoundRobinBalancer_WeightedDistribution(t *testing.T) {
	lb := NewRoundRobinBalancer()
	lb.AddBackend("gpt-4", Backend{
		ID:        "backend-3x",
		URL:       "http://localhost:8001",
		Weight:    3,
		ModelName: "gpt-4",
	})
	lb.AddBackend("gpt-4", Backend{
		ID:        "backend-1x",
		URL:       "http://localhost:8002",
		Weight:    1,
		ModelName: "gpt-4",
	})

	counts := map[string]int{"backend-3x": 0, "backend-1x": 0}
	n := 200
	for i := 0; i < n; i++ {
		b, _, ok := lb.Next("gpt-4", "")
		require.True(t, ok)
		counts[b.ID]++
		lb.ReleaseBackend(b.ID)
	}

	// 权重 3:1 → 3x 后端应该获得约 75% 的流量
	// 允许一定统计波动，但 3x 至少应该比 1x 多得多
	assert.Greater(t, counts["backend-3x"], counts["backend-1x"],
		"3x-weight backend should receive more traffic: 3x=%d, 1x=%d",
		counts["backend-3x"], counts["backend-1x"])
	// 3x 至少应该获得 50% 以上的流量（理论值 75%）
	assert.Greater(t, counts["backend-3x"], n/2,
		"3x-weight backend should get >50%% traffic: got %d/%d", counts["backend-3x"], n)
}

// TestRoundRobinBalancer_MarkFailedThreshold 验证 MarkFailed 需要连续 3 次失败才摘除后端
// 防止单次网络瞬断导致后端被长时间误封
func TestRoundRobinBalancer_MarkFailedThreshold(t *testing.T) {
	lb := NewRoundRobinBalancer()
	lb.AddBackend("gpt-4", Backend{ID: "backend-1", URL: "http://localhost:8001", ModelName: "gpt-4"})
	lb.AddBackend("gpt-4", Backend{ID: "backend-2", URL: "http://localhost:8002", ModelName: "gpt-4"})

	// 单次失败不应标记为不健康
	lb.MarkFailed("backend-1")

	// backend-1 应仍然可以被选中
	found := false
	for i := 0; i < 30; i++ {
		b, _, ok := lb.Next("gpt-4", "")
		require.True(t, ok)
		if b.ID == "backend-1" {
			found = true
		}
		lb.ReleaseBackend(b.ID)
	}
	assert.True(t, found, "backend-1 should still be selectable after 1 failure")

	// MarkSuccess 重置失败计数
	lb.MarkSuccess("backend-1")

	// 连续失败 3 次
	lb.MarkFailed("backend-1")
	lb.MarkFailed("backend-1")
	lb.MarkFailed("backend-1")

	// 现在 backend-1 应为不健康状态，所有请求都应发给 backend-2
	for i := 0; i < 10; i++ {
		b, _, ok := lb.Next("gpt-4", "")
		require.True(t, ok)
		assert.Equal(t, "backend-2", b.ID,
			"backend-1 should be unhealthy after 3 consecutive failures")
		lb.ReleaseBackend(b.ID)
	}

	// MarkSuccess 恢复后，backend-1 应该重新参与负载均衡
	lb.MarkSuccess("backend-1")
	found = false
	for i := 0; i < 30; i++ {
		b, _, ok := lb.Next("gpt-4", "")
		require.True(t, ok)
		if b.ID == "backend-1" {
			found = true
		}
		lb.ReleaseBackend(b.ID)
	}
	assert.True(t, found, "backend-1 should be selectable again after MarkSuccess")
}

// TestRoundRobinBalancer_MarkFailedResetOnSuccess 验证成功请求会重置失败计数
// 防止间歇性错误累积到阈值
func TestRoundRobinBalancer_MarkFailedResetOnSuccess(t *testing.T) {
	lb := NewRoundRobinBalancer()
	lb.AddBackend("gpt-4", Backend{ID: "backend-1", URL: "http://localhost:8001", ModelName: "gpt-4"})

	// 失败 2 次（未达到阈值 3）
	lb.MarkFailed("backend-1")
	lb.MarkFailed("backend-1")

	// 成功 1 次，重置计数
	lb.MarkSuccess("backend-1")

	// 再失败 2 次（计数器从 0 开始累积，而非从 2 继续）
	lb.MarkFailed("backend-1")
	lb.MarkFailed("backend-1")

	// backend-1 仍应健康（总失败 4 次但不连续，计数器已被 MarkSuccess 重置）
	b, _, ok := lb.Next("gpt-4", "")
	require.True(t, ok)
	assert.Equal(t, "backend-1", b.ID, "should still be healthy - MarkSuccess reset the counter")
}

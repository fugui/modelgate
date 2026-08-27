package proxy

import (
	"fmt"
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

// TestRoundRobinBalancer_StickySession_DeterministicHit 验证相同 sessionKey 稳定命中同一后端（KV Cache 亲和性）
func TestRoundRobinBalancer_StickySession_DeterministicHit(t *testing.T) {
	lb := NewRoundRobinBalancer()
	lb.AddBackend("gpt-4", Backend{ID: "node-1", URL: "http://10.0.0.1:8000", Weight: 1})
	lb.AddBackend("gpt-4", Backend{ID: "node-2", URL: "http://10.0.0.2:8000", Weight: 1})
	lb.AddBackend("gpt-4", Backend{ID: "node-3", URL: "http://10.0.0.3:8000", Weight: 1})

	sessionKey := "hdr:conv-opencode-98765"

	// 第一次获取确定目标后端
	firstBackend, _, ok := lb.Next("gpt-4", "", sessionKey)
	require.True(t, ok)
	lb.ReleaseBackend(firstBackend.ID)

	// 连续发起 50 次请求，每次释放后再次请求，必须 100% 命中同一个后端
	for i := 0; i < 50; i++ {
		b, _, ok := lb.Next("gpt-4", "", sessionKey)
		require.True(t, ok)
		assert.Equal(t, firstBackend.ID, b.ID, "Same sessionKey must strictly hit the exact same backend")
		lb.ReleaseBackend(b.ID)
	}
}

// TestRoundRobinBalancer_StickySession_Distribution 验证不同 sessionKey 能够相对均匀地分布在多个节点上
func TestRoundRobinBalancer_StickySession_Distribution(t *testing.T) {
	lb := NewRoundRobinBalancer()
	lb.AddBackend("gpt-4", Backend{ID: "node-1", URL: "http://10.0.0.1:8000", Weight: 1})
	lb.AddBackend("gpt-4", Backend{ID: "node-2", URL: "http://10.0.0.2:8000", Weight: 1})
	lb.AddBackend("gpt-4", Backend{ID: "node-3", URL: "http://10.0.0.3:8000", Weight: 1})

	hits := make(map[string]int)
	for i := 0; i < 300; i++ {
		sessionKey := fmt.Sprintf("ip:192.168.1.%d", i)
		b, _, ok := lb.Next("gpt-4", "", sessionKey)
		require.True(t, ok)
		hits[b.ID]++
		lb.ReleaseBackend(b.ID)
	}

	// 确保每个后端都分摊到了合理的流量
	assert.Equal(t, 3, len(hits))
	for nodeID, count := range hits {
		assert.True(t, count > 50, fmt.Sprintf("Node %s received %d hits, expected > 50", nodeID, count))
	}
}

// TestRoundRobinBalancer_StickySession_OverloadSpillover 验证首选后端满载时自动溢出保护到其他空闲后端
func TestRoundRobinBalancer_StickySession_OverloadSpillover(t *testing.T) {
	lb := NewRoundRobinBalancer()
	lb.AddBackend("gpt-4", Backend{ID: "node-1", URL: "http://10.0.0.1:8000", MaxConcurrency: 2, Weight: 1})
	lb.AddBackend("gpt-4", Backend{ID: "node-2", URL: "http://10.0.0.2:8000", MaxConcurrency: 2, Weight: 1})

	// 找到命中 node-1 的 sessionKey
	var sessionKeyNode1 string
	for i := 0; i < 100; i++ {
		testKey := fmt.Sprintf("session-%d", i)
		b, _, ok := lb.Next("gpt-4", "", testKey)
		require.True(t, ok)
		lb.ReleaseBackend(b.ID)
		if b.ID == "node-1" {
			sessionKeyNode1 = testKey
			break
		}
	}
	require.NotEmpty(t, sessionKeyNode1)

	// 请求 1：命中 node-1（不释放，模拟并发占用 1/2）
	b1, _, ok := lb.Next("gpt-4", "", sessionKeyNode1)
	require.True(t, ok)
	assert.Equal(t, "node-1", b1.ID)

	// 请求 2：命中 node-1（不释放，模拟并发占用 2/2，此时 node-1 已满载）
	b2, _, ok := lb.Next("gpt-4", "", sessionKeyNode1)
	require.True(t, ok)
	assert.Equal(t, "node-1", b2.ID)

	// 请求 3：node-1 已达到 MaxConcurrency=2 满载，此时再次发请求，不能报错，必须自动溢出到 node-2！
	b3, _, ok := lb.Next("gpt-4", "", sessionKeyNode1)
	require.True(t, ok)
	assert.Equal(t, "node-2", b3.ID, "When target node-1 is at capacity, sticky session must spill over to node-2")

	// 释放 node-1 的一个并发
	lb.ReleaseBackend(b1.ID)

	// 请求 4：node-1 恢复有空闲容量，新的请求重新回归命中首选 node-1
	b4, _, ok := lb.Next("gpt-4", "", sessionKeyNode1)
	require.True(t, ok)
	assert.Equal(t, "node-1", b4.ID, "When target node-1 recovers capacity, sticky session should hit node-1 again")

	lb.ReleaseBackend(b2.ID)
	lb.ReleaseBackend(b3.ID)
	lb.ReleaseBackend(b4.ID)
}

// TestRoundRobinBalancer_StickySession_UnhealthyFailover 验证首选后端故障时自动切换到健康节点
func TestRoundRobinBalancer_StickySession_UnhealthyFailover(t *testing.T) {
	lb := NewRoundRobinBalancer()
	lb.AddBackend("gpt-4", Backend{ID: "node-1", URL: "http://10.0.0.1:8000", Weight: 1})
	lb.AddBackend("gpt-4", Backend{ID: "node-2", URL: "http://10.0.0.2:8000", Weight: 1})

	sessionKey := "session-failover-test"
	preferred, _, ok := lb.Next("gpt-4", "", sessionKey)
	require.True(t, ok)
	lb.ReleaseBackend(preferred.ID)

	otherNodeID := "node-2"
	if preferred.ID == "node-2" {
		otherNodeID = "node-1"
	}

	// 模拟首选节点连续 3 次失败变为不健康
	lb.MarkFailed(preferred.ID)
	lb.MarkFailed(preferred.ID)
	lb.MarkFailed(preferred.ID)

	// 此时带有相同 sessionKey 的请求应自动切换到另一个健康的节点
	b, _, ok := lb.Next("gpt-4", "", sessionKey)
	require.True(t, ok)
	assert.Equal(t, otherNodeID, b.ID, "When preferred node is unhealthy, traffic must failover to healthy node")
	lb.ReleaseBackend(b.ID)
}


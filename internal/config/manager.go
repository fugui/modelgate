package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ConfigEvent 配置变更事件
type ConfigEvent struct {
	Type string      // "models", "policies", "all"
	Data interface{} // 可选的事件数据
}

// ConfigManager 配置管理器 - 管理配置的加载、保存和热重载
type ConfigManager struct {
	cfg      *Config
	path     string
	mu       sync.RWMutex
	watchers []chan<- ConfigEvent
	watchMu  sync.Mutex
}

// NewManager 创建新的配置管理器
func NewManager(cfg *Config, path string) *ConfigManager {
	return &ConfigManager{
		cfg:      cfg,
		path:     path,
		watchers: make([]chan<- ConfigEvent, 0),
	}
}

// GetConfig 获取当前配置的只读副本
func (cm *ConfigManager) GetConfig() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// 返回深拷贝以防止外部修改
	return cm.deepCopyConfig(cm.cfg)
}

// GetModels 获取模型配置列表
func (cm *ConfigManager) GetModels() []ModelConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	models := make([]ModelConfig, len(cm.cfg.Models))
	copy(models, cm.cfg.Models)
	return models
}

// GetPolicies 获取配额策略列表
func (cm *ConfigManager) GetPolicies() []PolicyConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	policies := make([]PolicyConfig, len(cm.cfg.Policies))
	copy(policies, cm.cfg.Policies)
	return policies
}

// Save 原子保存配置到文件
func (cm *ConfigManager) Save() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	return cm.saveLocked()
}

// saveLocked 内部保存方法（需要持有写锁）
func (cm *ConfigManager) saveLocked() error {
	// 1. 序列化配置
	data, err := yaml.Marshal(cm.cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// 2. 确保目录存在
	dir := filepath.Dir(cm.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// 3. 写入临时文件
	tmpPath := cm.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp config file: %w", err)
	}

	// 4. 如果原文件存在，进行备份
	backupPath := cm.path + ".backup"
	if _, err := os.Stat(cm.path); err == nil {
		if err := os.Rename(cm.path, backupPath); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("failed to backup config file: %w", err)
		}
	}

	// 5. 重命名临时文件为正式文件
	if err := os.Rename(tmpPath, cm.path); err != nil {
		// 尝试恢复备份
		os.Rename(backupPath, cm.path)
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename config file: %w", err)
	}

	// 6. 删除备份文件
	os.Remove(backupPath)

	return nil
}

// Subscribe 订阅配置变更事件
func (cm *ConfigManager) Subscribe() <-chan ConfigEvent {
	cm.watchMu.Lock()
	defer cm.watchMu.Unlock()

	ch := make(chan ConfigEvent, 10)
	cm.watchers = append(cm.watchers, ch)
	return ch
}

// Unsubscribe 取消订阅
func (cm *ConfigManager) Unsubscribe(ch <-chan ConfigEvent) {
	cm.watchMu.Lock()
	defer cm.watchMu.Unlock()

	for i, watcher := range cm.watchers {
		// Convert both to interface{} for comparison
		if interface{}(watcher) == interface{}(ch) {
			cm.watchers = append(cm.watchers[:i], cm.watchers[i+1:]...)
			close(watcher)
			break
		}
	}
}

// notifyWatchers 通知所有订阅者配置变更
func (cm *ConfigManager) notifyWatchers(event ConfigEvent) {
	cm.watchMu.Lock()
	defer cm.watchMu.Unlock()

	for _, ch := range cm.watchers {
		select {
		case ch <- event:
		default:
			// 如果通道已满，跳过此订阅者
		}
	}
}

// UpdateModels 更新模型配置
func (cm *ConfigManager) UpdateModels(models []ModelConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.cfg.Models = models
	if err := cm.saveLocked(); err != nil {
		return err
	}

	// 异步通知订阅者
	go cm.notifyWatchers(ConfigEvent{Type: "models", Data: models})
	return nil
}

// UpdatePolicies 更新配额策略配置
func (cm *ConfigManager) UpdatePolicies(policies []PolicyConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.cfg.Policies = policies
	if err := cm.saveLocked(); err != nil {
		return err
	}

	// 异步通知订阅者
	go cm.notifyWatchers(ConfigEvent{Type: "policies", Data: policies})
	return nil
}

// UpdateFrontend 更新前端配置
func (cm *ConfigManager) UpdateFrontend(frontend FrontendConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.cfg.Frontend = frontend
	if err := cm.saveLocked(); err != nil {
		return err
	}

	// 异步通知订阅者
	go cm.notifyWatchers(ConfigEvent{Type: "frontend", Data: frontend})
	return nil
}

// GetConcurrency 获取并发控制配置
func (cm *ConfigManager) GetConcurrency() ConcurrencyConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.cfg.Concurrency
}

// UpdateConcurrency 更新并发控制配置
func (cm *ConfigManager) UpdateConcurrency(concurrency ConcurrencyConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.cfg.Concurrency = concurrency
	if err := cm.saveLocked(); err != nil {
		return err
	}

	// 异步通知订阅者
	go cm.notifyWatchers(ConfigEvent{Type: "concurrency", Data: concurrency})
	return nil
}

// Reload 从文件重新加载配置（用于外部修改后）
func (cm *ConfigManager) Reload() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cfg, err := Load(cm.path)
	if err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}

	cm.cfg = cfg
	go cm.notifyWatchers(ConfigEvent{Type: "all", Data: cfg})
	return nil
}

// deepCopyConfig 深拷贝配置
func (cm *ConfigManager) deepCopyConfig(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}

	copy := &Config{
		Server:      cfg.Server,
		Database:    cfg.Database,
		JWT:         cfg.JWT,
		Admin:       cfg.Admin,
		Logs:        cfg.Logs,
		Frontend:    cfg.Frontend,
		Concurrency: cfg.Concurrency,
		SSO:         cfg.SSO,
	}

	// 深拷贝 Models
	if cfg.Models != nil {
		copy.Models = make([]ModelConfig, len(cfg.Models))
		for i, m := range cfg.Models {
			copy.Models[i] = cm.deepCopyModel(m)
		}
	}

	// 深拷贝 Policies
	if cfg.Policies != nil {
		copy.Policies = make([]PolicyConfig, len(cfg.Policies))
		for i, p := range cfg.Policies {
			copy.Policies[i] = p
		}
	}

	return copy
}

// deepCopyModel 深拷贝模型配置
func (cm *ConfigManager) deepCopyModel(m ModelConfig) ModelConfig {
	copy := ModelConfig{
		ID:            m.ID,
		Name:          m.Name,
		Description:   m.Description,
		Enabled:       m.Enabled,
		ContextWindow: m.ContextWindow,
		ModelParams:   make(map[string]interface{}),
	}

	// 深拷贝 ModelParams
	if m.ModelParams != nil {
		for k, v := range m.ModelParams {
			copy.ModelParams[k] = v
		}
	}

	// 深拷贝 Backends
	if m.Backends != nil {
		copy.Backends = make([]BackendConfig, len(m.Backends))
		for i, b := range m.Backends {
			copy.Backends[i] = b
		}
	}

	return copy
}

// GetModelByID 通过ID获取模型配置
func (cm *ConfigManager) GetModelByID(id string) *ModelConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, m := range cm.cfg.Models {
		if m.ID == id {
			copy := cm.deepCopyModel(m)
			return &copy
		}
	}
	return nil
}

// GetBackendByID 通过ID获取后端配置
func (cm *ConfigManager) GetBackendByID(backendID string) *BackendConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, m := range cm.cfg.Models {
		for _, b := range m.Backends {
			if b.ID == backendID {
				return &b
			}
		}
	}
	return nil
}

// GetBackendsByModel 获取指定模型的所有后端
func (cm *ConfigManager) GetBackendsByModel(modelID string) []BackendConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, m := range cm.cfg.Models {
		if m.ID == modelID {
			backends := make([]BackendConfig, len(m.Backends))
			copy(backends, m.Backends)
			return backends
		}
	}
	return nil
}

// GetPolicyByName 通过名称获取策略配置
func (cm *ConfigManager) GetPolicyByName(name string) *PolicyConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, p := range cm.cfg.Policies {
		if p.Name == name {
			return &p
		}
	}
	return nil
}

// AddModel 添加模型
func (cm *ConfigManager) AddModel(model ModelConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 检查是否已存在
	for _, m := range cm.cfg.Models {
		if m.ID == model.ID {
			return fmt.Errorf("model %s already exists", model.ID)
		}
	}

	cm.cfg.Models = append(cm.cfg.Models, model)
	if err := cm.saveLocked(); err != nil {
		return err
	}
	go cm.notifyWatchers(ConfigEvent{Type: "models", Data: cm.cfg.Models})
	return nil
}

// UpdateModel 更新模型
func (cm *ConfigManager) UpdateModel(model ModelConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	found := false
	for i, m := range cm.cfg.Models {
		if m.ID == model.ID {
			cm.cfg.Models[i] = model
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("model %s not found", model.ID)
	}

	if err := cm.saveLocked(); err != nil {
		return err
	}
	go cm.notifyWatchers(ConfigEvent{Type: "models", Data: cm.cfg.Models})
	return nil
}

// DeleteModel 删除模型
func (cm *ConfigManager) DeleteModel(modelID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	found := false
	newModels := make([]ModelConfig, 0, len(cm.cfg.Models))
	for _, m := range cm.cfg.Models {
		if m.ID == modelID {
			found = true
			continue
		}
		newModels = append(newModels, m)
	}

	if !found {
		return fmt.Errorf("model %s not found", modelID)
	}

	cm.cfg.Models = newModels
	if err := cm.saveLocked(); err != nil {
		return err
	}
	go cm.notifyWatchers(ConfigEvent{Type: "models", Data: cm.cfg.Models})
	return nil
}

// AddBackend 添加后端到模型
func (cm *ConfigManager) AddBackend(modelID string, backend BackendConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for i, m := range cm.cfg.Models {
		if m.ID == modelID {
			// 检查后端ID是否已存在
			for _, b := range m.Backends {
				if b.ID == backend.ID {
					return fmt.Errorf("backend %s already exists", backend.ID)
				}
			}

			cm.cfg.Models[i].Backends = append(cm.cfg.Models[i].Backends, backend)
			if err := cm.saveLocked(); err != nil {
				return err
			}
			go cm.notifyWatchers(ConfigEvent{Type: "models", Data: cm.cfg.Models})
			return nil
		}
	}

	return fmt.Errorf("model %s not found", modelID)
}

// UpdateBackend 更新后端
func (cm *ConfigManager) UpdateBackend(modelID string, backend BackendConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for i, m := range cm.cfg.Models {
		if m.ID == modelID {
			found := false
			for j, b := range m.Backends {
				if b.ID == backend.ID {
					cm.cfg.Models[i].Backends[j] = backend
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("backend %s not found", backend.ID)
			}
			if err := cm.saveLocked(); err != nil {
				return err
			}
			go cm.notifyWatchers(ConfigEvent{Type: "models", Data: cm.cfg.Models})
			return nil
		}
	}

	return fmt.Errorf("model %s not found", modelID)
}

// DeleteBackend 删除后端
func (cm *ConfigManager) DeleteBackend(modelID, backendID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for i, m := range cm.cfg.Models {
		if m.ID == modelID {
			found := false
			newBackends := make([]BackendConfig, 0, len(m.Backends))
			for _, b := range m.Backends {
				if b.ID == backendID {
					found = true
					continue
				}
				newBackends = append(newBackends, b)
			}
			if !found {
				return fmt.Errorf("backend %s not found", backendID)
			}
			cm.cfg.Models[i].Backends = newBackends
			if err := cm.saveLocked(); err != nil {
				return err
			}
			go cm.notifyWatchers(ConfigEvent{Type: "models", Data: cm.cfg.Models})
			return nil
		}
	}

	return fmt.Errorf("model %s not found", modelID)
}

// AddPolicy 添加配额策略
func (cm *ConfigManager) AddPolicy(policy PolicyConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 检查是否已存在
	for _, p := range cm.cfg.Policies {
		if p.Name == policy.Name {
			return fmt.Errorf("policy %s already exists", policy.Name)
		}
	}

	cm.cfg.Policies = append(cm.cfg.Policies, policy)
	if err := cm.saveLocked(); err != nil {
		return err
	}
	go cm.notifyWatchers(ConfigEvent{Type: "policies", Data: cm.cfg.Policies})
	return nil
}

// UpdatePolicy 更新配额策略
func (cm *ConfigManager) UpdatePolicy(policy PolicyConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	found := false
	for i, p := range cm.cfg.Policies {
		if p.Name == policy.Name {
			cm.cfg.Policies[i] = policy
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("policy %s not found", policy.Name)
	}

	if err := cm.saveLocked(); err != nil {
		return err
	}
	go cm.notifyWatchers(ConfigEvent{Type: "policies", Data: cm.cfg.Policies})
	return nil
}

// DeletePolicy 删除配额策略
func (cm *ConfigManager) DeletePolicy(name string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	found := false
	newPolicies := make([]PolicyConfig, 0, len(cm.cfg.Policies))
	for _, p := range cm.cfg.Policies {
		if p.Name == name {
			found = true
			continue
		}
		newPolicies = append(newPolicies, p)
	}

	if !found {
		return fmt.Errorf("policy %s not found", name)
	}

	cm.cfg.Policies = newPolicies
	if err := cm.saveLocked(); err != nil {
		return err
	}
	go cm.notifyWatchers(ConfigEvent{Type: "policies", Data: cm.cfg.Policies})
	return nil
}

// LastModified 获取配置文件最后修改时间
func (cm *ConfigManager) LastModified() (time.Time, error) {
	info, err := os.Stat(cm.path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// FileExists 检查配置文件是否存在
func (cm *ConfigManager) FileExists() bool {
	_, err := os.Stat(cm.path)
	return err == nil
}

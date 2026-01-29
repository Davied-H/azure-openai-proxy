package loadbalancer

import (
	"sync"
	"sync/atomic"
	"time"

	"azure-openai-proxy/config"
)

type BackendStatus struct {
	Backend     config.Backend
	Healthy     bool
	LastChecked time.Time
	FailCount   int32
}

type ModelBalancer struct {
	backends []*BackendStatus
	current  uint64
	mu       sync.RWMutex
}

type LoadBalancer struct {
	balancers map[string]*ModelBalancer
	mu        sync.RWMutex
}

var (
	instance *LoadBalancer
	once     sync.Once
)

// GetInstance 获取负载均衡器单例
func GetInstance() *LoadBalancer {
	once.Do(func() {
		instance = &LoadBalancer{
			balancers: make(map[string]*ModelBalancer),
		}
	})
	return instance
}

// Init 初始化负载均衡器
func (lb *LoadBalancer) Init(cfg *config.Config) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	for model, modelCfg := range cfg.Models {
		balancer := &ModelBalancer{
			backends: make([]*BackendStatus, len(modelCfg.Backends)),
		}
		for i, backend := range modelCfg.Backends {
			balancer.backends[i] = &BackendStatus{
				Backend: backend,
				Healthy: true,
			}
		}
		lb.balancers[model] = balancer
	}
}

// GetNext 获取下一个可用后端（轮询）
func (lb *LoadBalancer) GetNext(model string) *BackendStatus {
	lb.mu.RLock()
	balancer, ok := lb.balancers[model]
	lb.mu.RUnlock()

	if !ok || len(balancer.backends) == 0 {
		return nil
	}

	// 轮询选择
	n := len(balancer.backends)
	for i := 0; i < n; i++ {
		idx := atomic.AddUint64(&balancer.current, 1) % uint64(n)
		backend := balancer.backends[idx]

		balancer.mu.RLock()
		healthy := backend.Healthy
		balancer.mu.RUnlock()

		if healthy {
			return backend
		}
	}

	// 所有后端都不健康时，返回第一个尝试
	return balancer.backends[0]
}

// GetAllBackends 获取模型的所有后端（用于故障转移）
func (lb *LoadBalancer) GetAllBackends(model string) []*BackendStatus {
	lb.mu.RLock()
	balancer, ok := lb.balancers[model]
	lb.mu.RUnlock()

	if !ok {
		return nil
	}

	// 返回从当前位置开始的后端列表（用于故障转移顺序）
	n := len(balancer.backends)
	if n == 0 {
		return nil
	}

	result := make([]*BackendStatus, n)
	startIdx := atomic.LoadUint64(&balancer.current) % uint64(n)

	for i := 0; i < n; i++ {
		idx := (int(startIdx) + i) % n
		result[i] = balancer.backends[idx]
	}

	return result
}

// MarkUnhealthy 标记后端为不健康
func (lb *LoadBalancer) MarkUnhealthy(model string, backend *BackendStatus) {
	lb.mu.RLock()
	balancer, ok := lb.balancers[model]
	lb.mu.RUnlock()

	if !ok {
		return
	}

	balancer.mu.Lock()
	defer balancer.mu.Unlock()

	backend.Healthy = false
	backend.LastChecked = time.Now()
	backend.FailCount++
}

// MarkHealthy 标记后端为健康
func (lb *LoadBalancer) MarkHealthy(model string, backend *BackendStatus) {
	lb.mu.RLock()
	balancer, ok := lb.balancers[model]
	lb.mu.RUnlock()

	if !ok {
		return
	}

	balancer.mu.Lock()
	defer balancer.mu.Unlock()

	backend.Healthy = true
	backend.LastChecked = time.Now()
	backend.FailCount = 0
}

const defaultRecoveryTimeout = 30 * time.Second

// StartHealthCheck 启动健康检查（定期恢复不健康的后端）
func (lb *LoadBalancer) StartHealthCheck(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			// 先复制 balancers map，避免长时间持有读锁
			lb.mu.RLock()
			balancersCopy := make([]*ModelBalancer, 0, len(lb.balancers))
			for _, b := range lb.balancers {
				balancersCopy = append(balancersCopy, b)
			}
			lb.mu.RUnlock()

			// 逐个处理 balancer
			for _, balancer := range balancersCopy {
				balancer.mu.Lock()
				for _, backend := range balancer.backends {
					// 超时后自动恢复健康状态以便重试
					if !backend.Healthy && time.Since(backend.LastChecked) > defaultRecoveryTimeout {
						backend.Healthy = true
					}
				}
				balancer.mu.Unlock()
			}
		}
	}()
}

// HasModel 检查是否配置了指定模型
func (lb *LoadBalancer) HasModel(model string) bool {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	_, ok := lb.balancers[model]
	return ok
}

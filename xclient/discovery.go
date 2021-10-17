package xclient

import (
	"errors"
	"math"
	"math/rand"
	"sync"
	"time"
)

type SelectMode int

const (
	RandomSelect     SelectMode = iota // 随机选择策略
	RoundRobinSelect                   // Round-Robin 轮询算法
)

type Discovery interface {
	Refresh() error                      // 从注册中心更新服务列表
	Update(servers []string) error       // 手动更新服务列表
	Get(mode SelectMode) (string, error) // 根据负载均衡策略, 选择一个服务实例
	GetAll() ([]string, error)           // 返回所有的服务实例
}

type MultiServersDiscovery struct {
	r       *rand.Rand   // 生成随机数
	mu      sync.RWMutex // 读写锁, 保护对注册地址的读写
	servers []string     // 实例地址列表
	index   int          // Round-Robin 轮询算法中需要记录上一次选择的位置
}

// Refresh 从注册中心更新服务列表
func (m *MultiServersDiscovery) Refresh() error {
	return nil
}

// Update 手动更新服务列表
func (m *MultiServersDiscovery) Update(servers []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers = servers
	return nil
}

// Get 根据负载均衡策略, 选择一个服务实例
func (m *MultiServersDiscovery) Get(mode SelectMode) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(m.servers)
	if n == 0 {
		return "", errors.New("rpc discovery: 没有发现可以访问的实例")
	}
	switch mode {
	case RandomSelect:
		return m.servers[m.r.Intn(n)], nil
	case RoundRobinSelect:
		s := m.servers[m.index%n]
		m.index = (m.index + 1) % n
		return s, nil
	default:
		return "", errors.New("rpc discovery: 不支持给定的负载均衡模式")
	}
}

// GetAll 返回所有的服务实例
func (m *MultiServersDiscovery) GetAll() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// 拷贝一份新的实例列表, 避免误修改
	servers := make([]string, len(m.servers))
	copy(servers, m.servers)
	return servers, nil
}

// NewMultiServersDiscovery 新建服务发现
func NewMultiServersDiscovery(servers []string) *MultiServersDiscovery {
	d := &MultiServersDiscovery{
		servers: servers,
		r:       rand.New(rand.NewSource(time.Now().UnixNano())), // 初始化时使用时间戳设定随机数种子
	}
	d.index = d.r.Intn(math.MaxInt32 - 1)
	return d
}

var _ Discovery = (*MultiServersDiscovery)(nil)

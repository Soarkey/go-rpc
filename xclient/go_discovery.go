package xclient

import (
	"log"
	"net/http"
	"strings"
	"time"
)

type GoRegistryDiscovery struct {
	*MultiServersDiscovery
	registry   string        // 注册中心地址
	timeout    time.Duration // 超时时间
	lastUpdate time.Time     // 上次更新时间
}

// Update 手动更新服务列表
func (r *GoRegistryDiscovery) Update(servers []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.servers = servers
	r.lastUpdate = time.Now()
	return nil
}

// Refresh 从注册中心更新服务列表
func (r *GoRegistryDiscovery) Refresh() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.lastUpdate.Add(r.timeout).After(time.Now()) {
		// 未过期, 无需刷新
		return nil
	}
	// 超时, 重新获取服务列表
	log.Println("rpc registry: 刷新注册中心服务列表 ", r.registry)
	resp, err := http.Get(r.registry)
	if err != nil {
		log.Println("rpc registry: 注册中心刷新出现错误 ", err)
		return err
	}
	// 获取所有在线的服务列表
	servers := strings.Split(resp.Header.Get("X-Gorpc-Servers"), ",")
	r.servers = make([]string, 0, len(servers))
	for _, s := range servers {
		s = strings.TrimSpace(s)
		if s != "" {
			r.servers = append(r.servers, s)
		}
	}
	r.lastUpdate = time.Now()
	return nil
}

// Get 根据负载均衡策略, 选择一个服务实例
func (r *GoRegistryDiscovery) Get(mode SelectMode) (string, error) {
	// 获取服务前需要进行一次刷新, 确保服务列表没有过期
	if err := r.Refresh(); err != nil {
		return "", err
	}
	return r.MultiServersDiscovery.Get(mode)
}

// GetAll 返回所有的服务实例
func (r *GoRegistryDiscovery) GetAll() ([]string, error) {
	// 获取服务前需要进行一次刷新, 确保服务列表没有过期
	if err := r.Refresh(); err != nil {
		return nil, err
	}
	return r.MultiServersDiscovery.GetAll()
}

const defaultUpdateTimeout = 1 * time.Second

func NewGoRegistryDiscovery(registry string, timeout time.Duration) *GoRegistryDiscovery {
	if timeout == 0 {
		timeout = defaultUpdateTimeout
	}
	return &GoRegistryDiscovery{
		MultiServersDiscovery: NewMultiServersDiscovery(make([]string, 0)),
		registry:              registry,
		timeout:               timeout,
	}
}

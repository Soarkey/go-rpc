package registry

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type ServerItem struct {
	Addr  string    // 实例地址
	start time.Time // 启动时间
}

type GoRegistry struct {
	timeout time.Duration          // 超时时间
	mu      sync.Mutex             // 保护操作的完整性
	servers map[string]*ServerItem // 实例列表
}

// putServer 添加服务实例, 如果服务已存在则更新启动时间 start
func (r *GoRegistry) putServer(addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.servers[addr]
	if s == nil {
		// 服务不存在, 执行新增, 设置启动时间
		r.servers[addr] = &ServerItem{Addr: addr, start: time.Now()}
		return
	}
	// 若服务已存在, 更新启动时间保活
	s.start = time.Now()
}

// aliveServers 返回可用的服务列表, 如果存在超时的服务则删除
func (r *GoRegistry) aliveServers() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var alive []string
	for addr, s := range r.servers {
		if r.timeout == 0 || s.start.Add(r.timeout).After(time.Now()) {
			// 未设置超时时间 或者 启动时间加上超时时间大与当前时刻, 表明服务未过期, 添加到活跃服务列表
			alive = append(alive, addr)
			continue
		}
		// 服务已超时, 进行删除
		delete(r.servers, addr)
	}
	sort.Strings(alive)
	return alive
}

// ServeHTTP 运行在/_geerpc_/registry
func (r *GoRegistry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		w.Header().Set("X-Gorpc-Servers", strings.Join(r.aliveServers(), ","))
	case "POST":
		addr := req.Header.Get("X-Gorpc-Servers")
		if addr == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.putServer(addr)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *GoRegistry) HandleHTTP(registryPath string) {
	http.Handle(registryPath, r)
	log.Println("rpc registry path: ", registryPath)
}

const (
	defaultPath    = "/_gorpc_/registry" // 注册路径
	defaultTimeout = 5 * time.Minute     // 默认超时时间为5min
)

func NewRegistry(timeout time.Duration) *GoRegistry {
	return &GoRegistry{
		timeout: timeout,
		servers: make(map[string]*ServerItem),
	}
}

var DefaultGoRegistry = NewRegistry(defaultTimeout)

func HandleHTTP() {
	DefaultGoRegistry.HandleHTTP(defaultPath)
}

// Heartbeat 发送心跳消息保活
func Heartbeat(registry, addr string, duration time.Duration) {
	if duration == 0 {
		// 确保在被移除之前有足够时间发送心跳
		duration = time.Second // defaultTimeout - time.Duration(1)*time.Minute
	}
	// 利用定时器定时发送心跳
	go func() {
		var err error
		err = sendHeartbeat(registry, addr)
		t := time.NewTicker(duration)
		defer t.Stop()
		for err != nil {
			<-t.C
			err = sendHeartbeat(registry, addr)
		}
	}()
}

func sendHeartbeat(registry, addr string) error {
	log.Println(addr, " 发送心跳消息到注册中心 ", registry)
	httpClient := &http.Client{}
	req, _ := http.NewRequest("POST", registry, nil)
	req.Header.Set("X-Gorpc-Servers", addr)
	if _, err := httpClient.Do(req); err != nil {
		log.Println("rpc server: 发送心跳消息出错 ", err)
		return err
	}
	return nil
}

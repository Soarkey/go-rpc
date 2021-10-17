package xclient

import (
	"context"
	"io"
	"reflect"
	"sync"

	"gorpc/client"
	"gorpc/option"
)

type XClient struct {
	d       Discovery
	mode    SelectMode
	opt     *option.Option
	mu      sync.Mutex
	clients map[string]*client.Client
}

func (xc *XClient) Close() error {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	for key, c := range xc.clients {
		_ = c.Close()
		delete(xc.clients, key)
	}
	return nil
}

// dial 复用 Client 进行调用
// 1.检查 xc.clients 是否有缓存的 Client,
//   如果有, 检查是否是可用状态, 如果是则返回缓存的 Client
//   如果不可用, 则从缓存中删除
// 2.如果步骤 1 没有返回缓存的 Client, 则说明需要创建新的 Client 缓存并返回
func (xc *XClient) dial(rpcAddr string) (*client.Client, error) {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	c, ok := xc.clients[rpcAddr]
	if ok && !c.IsAvailable() {
		_ = c.Close()
		delete(xc.clients, rpcAddr)
		c = nil
	}
	if c == nil {
		var err error
		c, err = client.XDial(rpcAddr, xc.opt)
		if err != nil {
			return nil, err
		}
		xc.clients[rpcAddr] = c
	}
	return c, nil
}

func (xc *XClient) call(rpcAddr string, ctx context.Context, serviceMethod string, args, reply interface{}) error {
	c, err := xc.dial(rpcAddr)
	if err != nil {
		return err
	}
	return c.Call(ctx, serviceMethod, args, reply)
}

func (xc *XClient) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	rpcAddr, err := xc.d.Get(xc.mode)
	if err != nil {
		return err
	}
	return xc.call(rpcAddr, ctx, serviceMethod, args, reply)
}

// Broadcast 广播到所有的服务实例
// 如果任意一个实例发生错误, 则返回其中一个错误, 并取消 context 传播
// 如果调用成功, 则返回其中一个的结果
func (xc *XClient) Broadcast(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	servers, err := xc.d.GetAll()
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	var mu sync.Mutex // 保护 e 和 replyDone
	var e error
	replyDone := reply == nil
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// 并发请求所有服务实例
	for _, rpcAddr := range servers {
		wg.Add(1)
		go func(rpcAddr string) {
			defer wg.Done()
			var clonedReply interface{}
			if reply != nil {
				clonedReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
			}
			err := xc.call(rpcAddr, ctx, serviceMethod, args, clonedReply)
			mu.Lock()
			// 快速失败, 若失败马上取消 context
			if err != nil && e == nil {
				e = err
				cancel()
			}
			if err == nil && !replyDone {
				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(clonedReply).Elem())
				replyDone = true
			}
			mu.Unlock()
		}(rpcAddr)
	}
	wg.Wait()
	return e
}

var _ io.Closer = (*XClient)(nil)

// NewXClient 创建一个支持负载均衡的客户端
// 接收参数: 服务发现实例 Discovery, 负载均衡模式 SelectMode 以及协议选项 option.Option
func NewXClient(d Discovery, mode SelectMode, opt *option.Option) *XClient {
	return &XClient{d: d, mode: mode, opt: opt, clients: make(map[string]*client.Client)}
}

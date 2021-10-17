package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"gorpc/registry"
	"gorpc/server"
	"gorpc/xclient"
)

type Foo int
type Args struct {
	Num1, Num2 int
}

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func (f Foo) Sleep(args Args, reply *int) error {
	time.Sleep(time.Duration(args.Num1) * time.Second)
	*reply = args.Num1 + args.Num2
	return nil
}

func startRegistry(wg *sync.WaitGroup) {
	lis, _ := net.Listen("tcp", ":9999")
	registry.HandleHTTP()
	wg.Done()
	_ = http.Serve(lis, nil)
}

func startServer(registryAddr string, wg *sync.WaitGroup) {
	// 注意: 在 startServer 中使用了信道 addr, 确保服务端端口监听成功, 客户端再发起请求
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalln("网络出现错误 err: ", err)
	}
	s := server.NewServer()
	var foo Foo
	_ = s.Register(&foo)
	registry.Heartbeat(registryAddr, "tcp@"+lis.Addr().String(), 0)
	log.Println("启动rpc服务器 ", lis.Addr())
	wg.Done()
	s.Accept(lis)
}

func foo(xc *xclient.XClient, ctx context.Context, typ, serviceMethod string, args *Args) {
	var (
		reply int
		err   error
	)
	switch typ {
	case "call":
		err = xc.Call(ctx, serviceMethod, args, &reply)
	case "broadcast":
		err = xc.Broadcast(ctx, serviceMethod, args, &reply)
	}
	if err != nil {
		log.Printf("%s %s error: %v", typ, serviceMethod, err)
		return
	}
	log.Printf("%s %s success: %d + %d = %d", typ, serviceMethod, args.Num1, args.Num2, reply)
}

// simulateCall 模拟调用
func simulateCall(registry, typ string, owg *sync.WaitGroup) {
	// 创建服务注册中心
	d := xclient.NewGoRegistryDiscovery(registry, 0)
	// 创建带负载均衡的客户端
	xc := xclient.NewXClient(d, xclient.RandomSelect, nil)
	defer func() {
		_ = xc.Close()
	}()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			foo(xc, context.Background(), typ, "Foo.Sum", &Args{Num1: i, Num2: i + 1})
			if typ == "broadcast" {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				foo(xc, ctx, typ, "Foo.Sleep", &Args{Num1: i, Num2: i + 1})
			}
		}(i)
	}
	wg.Wait()
	if owg != nil {
		owg.Done()
	}
}

func main() {
	log.SetFlags(0)
	registryAddr := "http://localhost:9999/_gorpc_/registry"
	// 启动注册中心
	var wg sync.WaitGroup
	wg.Add(1)
	go startRegistry(&wg)
	wg.Wait()
	time.Sleep(2 * time.Second)

	// 启动多个服务
	wg.Add(3)
	go startServer(registryAddr, &wg)
	go startServer(registryAddr, &wg)
	go startServer(registryAddr, &wg)
	wg.Wait()

	time.Sleep(2 * time.Second)

	wg.Add(1)
	go simulateCall(registryAddr, "call", &wg)
	wg.Wait()
	time.Sleep(2 * time.Second)
	fmt.Println("-----------------------------------------------")
	wg.Add(1)
	simulateCall(registryAddr, "broadcast", &wg)
	wg.Wait()
}

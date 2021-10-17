package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

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

func startServer(addr chan string) {
	// 注意: 在 startServer 中使用了信道 addr, 确保服务端端口监听成功, 客户端再发起请求
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalln("网络出现错误 err: ", err)
	}
	s := server.NewServer()
	var foo Foo
	_ = s.Register(&foo)
	log.Println("启动rpc服务器 ", lis.Addr())
	addr <- lis.Addr().String()
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
func simulateCall(addr []string, typ string) {
	for i := range addr {
		addr[i] = "tcp@" + addr[i]
	}
	// 手动注册服务
	d := xclient.NewMultiServersDiscovery(addr)
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
}

func main() {
	log.SetFlags(0)
	ch1 := make(chan string)
	ch2 := make(chan string)
	// 启动两个服务
	go startServer(ch1)
	go startServer(ch2)
	addr1 := <-ch1
	addr2 := <-ch2

	done := make(chan struct{})
	fmt.Println("-----------------  call  -----------------")
	time.Sleep(time.Second)
	go func() {
		simulateCall([]string{addr1, addr2}, "call")
		done <- struct{}{}
	}()
	<-done
	fmt.Println("-----------------  broadcast  -----------------")
	time.Sleep(time.Second)
	simulateCall([]string{addr1, addr2}, "broadcast")
}

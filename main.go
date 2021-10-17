package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"gorpc/client"
	"gorpc/server"
)

type Foo int
type Args struct {
	Num1, Num2 int
}

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func startServer(addr chan string) {
	// 注意: 在 startServer 中使用了信道 addr, 确保服务端端口监听成功, 客户端再发起请求
	lis, err := net.Listen("tcp", ":9999")
	if err != nil {
		log.Fatalln("网络出现错误 err: ", err)
	}
	var foo Foo
	if err = server.Register(&foo); err != nil {
		log.Fatalln("服务注册错误 ", err)
	}
	server.HandleHTTP()
	log.Println("启动rpc服务器 ", lis.Addr())
	addr <- lis.Addr().String()
	if err = http.Serve(lis, nil); err != nil {
		log.Fatalln("监听出现错误 ", err)
	}
}

func call(addr chan string) {
	c, err := client.DialHTTP("tcp", <-addr)
	defer func() {
		_ = c.Close()
		if err != nil {
			log.Fatalln("tcp拨号连接失败 err: ", err)
		}
	}()
	time.Sleep(time.Second)
	// 发送请求 & 接收响应
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := &Args{Num1: i, Num2: i + 1}
			var reply int
			if err = c.Call(context.Background(), "Foo.Sum", args, &reply); err != nil {
				log.Fatalln("调用 Foo.Sum 出错 err: ", err)
			}
			log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
		}(i)
		time.Sleep(time.Second)
	}
	wg.Wait()
}

func main() {
	log.SetFlags(0)
	ch := make(chan string)
	go call(ch)
	startServer(ch)
}

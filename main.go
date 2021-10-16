package main

import (
	"log"
	"net"
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
	var foo Foo
	if err := server.Register(&foo); err != nil {
		log.Fatalln("服务注册错误 ", err)
	}
	// 注意: 在 startServer 中使用了信道 addr, 确保服务端端口监听成功, 客户端再发起请求
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalln("网络出现错误 err: ", err)
	}
	log.Println("启动rpc服务器 ", lis.Addr())
	addr <- lis.Addr().String()
	server.Accept(lis)
}

func main() {
	log.SetFlags(0)
	addr := make(chan string)
	go startServer(addr)

	client, err := client.Dial("tcp", <-addr)
	defer func() {
		_ = client.Close()
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
			if err = client.Call("Foo.Sum", args, &reply); err != nil {
				log.Fatalln("调用 Foo.Sum 出错 err: ", err)
			}
			log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
		}(i)
	}
	wg.Wait()
	log.Println("程序运行完毕!")
}

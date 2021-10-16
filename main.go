package main

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"gorpc/app"
)

func startServer(addr chan string) {
	// 注意: 在 startServer 中使用了信道 addr, 确保服务端端口监听成功, 客户端再发起请求
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalln("网络出现错误 err: ", err)
	}
	log.Println("启动rpc服务器 ", lis.Addr())
	addr <- lis.Addr().String()
	app.Accept(lis)
}

func main() {
	addr := make(chan string)
	go startServer(addr)

	client, err := app.Dial("tcp", <-addr)
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
			args := fmt.Sprintf("rpc req %d", i)
			var reply string
			if err = client.Call("Foo.Sum", args, &reply); err != nil {
				log.Fatalln("call Foo.Sum error: ", err)
			}
			log.Println("reply: ", reply)
		}(i)
	}
	wg.Wait()
	log.Println("程序运行完毕!")
}

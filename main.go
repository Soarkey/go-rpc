package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"

	"gorpc/codec"
)

func startServer(addr chan string) {
	// 注意: 在 startServer 中使用了信道 addr, 确保服务端端口监听成功, 客户端再发起请求
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalln("网络出现错误 err: ", err)
	}
	log.Println("启动rpc服务器 ", lis.Addr())
	addr <- lis.Addr().String()
	Accept(lis)
}

func main() {
	addr := make(chan string)
	go startServer(addr)

	conn, err := net.Dial("tcp", <-addr)
	if err != nil {
		log.Fatalln("tcp拨号连接失败 err: ", err)
		return
	}
	defer func() {
		_ = conn.Close()
	}()

	time.Sleep(time.Second)

	// 发送Option
	_ = json.NewEncoder(conn).Encode(DefaultOption)
	c := codec.NewGobCodec(conn)

	log.Println("发送Option: ", DefaultOption)

	// 发送请求 & 接收响应
	for i := 0; i < 5; i++ {
		h := &codec.Header{
			ServiceMethod: "Foo.Sum",
			Seq:           uint64(i),
		}

		_ = c.Write(h, fmt.Sprintf("gorpc req %d", h.Seq))
		_ = c.ReadHeader(h)

		var reply string
		_ = c.ReadBody(&reply)

		log.Println("响应信息: ", reply)
	}

	log.Println("程序运行完毕!")
}

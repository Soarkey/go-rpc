package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
	"sync"

	"gorpc/codec"
)

// MagicNumber 魔数
const MagicNumber = 0x3bef5c

type Option struct {
	MagicNumber int        // 魔数, 标记这是一个go-rpc自定义请求
	CodecType   codec.Type // client端可以选择不同的Codec来编码body
}

var DefaultOption = &Option{
	MagicNumber: MagicNumber,
	CodecType:   codec.GobType,
}

// | Option{MagicNumber: xxx, CodecType: xxx} | Header{ServiceMethod ...} | Body interface{} |
// | <------      固定 JSON 编码      ------>  | <-------   编码方式由 CodeType 决定   ------->|

// Server 表示一个RPC服务器
type Server struct{}

// Accept 接收 net.Listener 中的连接
func (s *Server) Accept(lis net.Listener) {
	// for 循环等待 socket 连接建立, 并开启 ServerConn 子协程处理
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Fatalln("rpc server: 接收连接出错 err: ", err)
			return
		}
		go s.ServeConn(conn)
	}
}

// ServeConn 处理单个连接请求
func (s *Server) ServeConn(conn io.ReadWriteCloser) {
	defer func() {
		_ = conn.Close()
	}()

	var opt Option
	// 首先使用 json.NewDecoder 反序列化得到 Option 实例
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Fatalln("rpc server: 反序列化Option出错 err: ", err)
		return
	}
	// 检查 MagicNumber 和 CodeType 的值是否正确,
	// 根据 CodeType 得到对应的消息编解码器
	if opt.MagicNumber != MagicNumber {
		log.Fatalf("rpc server: 非法魔数 %x\n", opt.MagicNumber)
		return
	}
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Fatalf("rpc server: 非法CodecType %s\n", opt.CodecType)
		return
	}
	s.serveCodec(f(conn))
}

// invalidRequest 当响应argv出错时设置
var invalidRequest = struct{}{}

// request 存储一次请求的所有信息
type request struct {
	h            *codec.Header
	argv, replyv reflect.Value
}

// serveCodec 解析消息
// 注意:
// 1.handleRequest 使用了协程并发执行请求
// 2.处理请求是并发的, 但是回复请求的报文必须是逐个发送的, 并发容易导致多个回复报文交织在一起, 客户端无法解析, 在这里使用锁(sending)保证
// 3.尽力而为, 只有在 header 解析失败时, 才终止循环
func (s *Server) serveCodec(c codec.Codec) {
	// 加锁确保发送一条完整的消息
	sending := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	// 一次连接 可能对应多个请求头和请求体
	// | Option | Header1 | Body1 | Header2 | Body2 | ...
	for {
		// 读取请求
		req, err := s.readRequest(c)
		if err != nil {
			if req == nil {
				break
			}
			req.h.Error = err.Error()
			s.sendResponse(c, req.h, invalidRequest, sending)
			continue
		}
		wg.Add(1)
		// 处理请求
		go s.handleRequest(c, req, sending, wg)
	}
	wg.Wait()
	_ = c.Close()
}

// handleRequest 处理请求
func (s *Server) handleRequest(c codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Printf("请求信息:\n >> 请求头 %v \n >> 请求体 %v\n", req.h, req.argv.Elem())
	req.replyv = reflect.ValueOf(fmt.Sprintf("go-rpc resp %d", req.h.Seq))
	s.sendResponse(c, req.h, req.replyv.Interface(), sending)
}

// readRequest 读取请求
func (s *Server) readRequest(c codec.Codec) (*request, error) {
	h, err := s.readRequestHeader(c)
	if err != nil {
		return nil, err
	}
	req := &request{h: h}
	req.argv = reflect.New(reflect.TypeOf(""))
	if err = c.ReadBody(req.argv.Interface()); err != nil {
		log.Fatalln("rpc server: 读取argv出错 err: ", err)
	}
	return req, nil
}

func (s *Server) readRequestHeader(c codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := c.ReadHeader(&h); err != nil {
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Fatalln("rpc server: 读取header出错 err: ", err)
		}
		return nil, err
	}
	return &h, nil
}

// sendResponse 回复请求
func (s *Server) sendResponse(c codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := c.Write(h, body); err != nil {
		log.Fatalln("rpc server: 写入响应信息出错 err: ", err)
	}
}

// NewServer 返回一个新的 Server
func NewServer() *Server {
	return &Server{}
}

// DefaultServer 默认的 *Server 实例
var DefaultServer = NewServer()

// Accept 接收连接并响应请求
func Accept(lis net.Listener) {
	DefaultServer.Accept(lis)
}

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"gorpc/codec"
	"gorpc/option"
)

// | Option{MagicNumber: xxx, CodecType: xxx} | Header{ServiceMethod ...} | Body interface{} |
// | <------      固定 JSON 编码      ------>  | <-------   编码方式由 CodeType 决定   ------->|

// Server 表示一个RPC服务器
type Server struct {
	serviceMap sync.Map
}

// Register 服务注册
func (s *Server) Register(rcvr interface{}) error {
	svc := newService(rcvr)
	if _, dup := s.serviceMap.LoadOrStore(svc.name, svc); dup {
		return errors.New("rpc server: 服务已经被定义 " + svc.name)
	}
	return nil
}

// findService 通过服务名找到对应的方法
func (s *Server) findService(serviceMethod string) (svc *service, mtype *methodType, err error) {
	// serviceMethod的组成为 Service.Method
	// 根据 . 分割两个部分
	dot := strings.LastIndex(serviceMethod, ".")
	if dot < 0 {
		err = errors.New("rpc server: service/method 请求不符合格式 " + serviceMethod)
		return
	}
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]
	svci, ok := s.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc server: 找不到服务 " + serviceName)
		return
	}
	svc = svci.(*service)
	mtype = svc.method[methodName]
	if mtype == nil {
		err = errors.New("rpc server: 找不到方法 " + serviceName)
	}
	return
}

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
	var opt option.Option
	// 首先使用 json.NewDecoder 反序列化得到 Option 实例
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Fatalln("rpc server: 反序列化Option出错 err: ", err)
		return
	}
	// 检查 MagicNumber 和 CodeType 的值是否正确,
	// 根据 CodeType 得到对应的消息编解码器
	if opt.MagicNumber != option.MagicNumber {
		log.Fatalf("rpc server: 非法魔数 %x\n", opt.MagicNumber)
		return
	}
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Fatalf("rpc server: 非法CodecType %s\n", opt.CodecType)
		return
	}
	s.serveCodec(f(conn), &opt)
}

// invalidRequest 当响应argv出错时设置
var invalidRequest = struct{}{}

// request 存储一次请求的所有信息
type request struct {
	h            *codec.Header // 请求头
	argv, replyv reflect.Value // 参数和返回值
	mtype        *methodType
	svc          *service
}

// serveCodec 解析消息
// 注意:
// 1.handleRequest 使用了协程并发执行请求
// 2.处理请求是并发的, 但是回复请求的报文必须是逐个发送的, 并发容易导致多个回复报文交织在一起, 客户端无法解析, 在这里使用锁(sending)保证
// 3.尽力而为, 只有在 header 解析失败时, 才终止循环
func (s *Server) serveCodec(c codec.Codec, opt *option.Option) {
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
		go s.handleRequest(c, req, sending, wg, opt.HandleTimeout)
	}
	wg.Wait()
	_ = c.Close()
}

// handleRequest 处理请求
func (s *Server) handleRequest(c codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup, timeout time.Duration) {
	defer wg.Done()
	called := make(chan struct{})
	sent := make(chan struct{})
	go func() {
		err := req.svc.call(req.mtype, req.argv, req.replyv)
		log.Printf("请求信息: >> [请求头 %v / 请求体 %v]\n", req.h, req.argv)
		called <- struct{}{}
		if err != nil {
			req.h.Error = err.Error()
			s.sendResponse(c, req.h, invalidRequest, sending)
			sent <- struct{}{}
			return
		}
		s.sendResponse(c, req.h, req.replyv.Interface(), sending)
		sent <- struct{}{}
	}()
	if timeout == 0 {
		<-called
		<-sent
		return
	}
	select {
	case <-time.After(timeout):
		req.h.Error = fmt.Sprintf("rpc server: 处理请求超时, 超时时间为%s", timeout)
		s.sendResponse(c, req.h, invalidRequest, sending)
	case <-called:
		<-sent
	}
}

// readRequest 读取请求
func (s *Server) readRequest(c codec.Codec) (*request, error) {
	h, err := s.readRequestHeader(c)
	if err != nil {
		return nil, err
	}
	req := &request{h: h}
	req.svc, req.mtype, err = s.findService(h.ServiceMethod)
	if err != nil {
		return req, err
	}
	// 构造参数
	req.argv = req.mtype.newArgv()
	req.replyv = req.mtype.newReplyv()
	argvi := req.argv.Interface()
	// 确保 argvi 是指针类型, ReadBody 需要指针作为参数
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}
	if err = c.ReadBody(argvi); err != nil {
		log.Fatalln("rpc server: 读取argv出错 err: ", err)
		return req, err
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

// Register 服务注册
func Register(rcvr interface{}) error {
	return DefaultServer.Register(rcvr)
}

const (
	CONNECTED        = "200 Connected to go-rpc"
	DefaultRpcPath   = "/_gorpc_"
	DefaultDebugPath = "/debug/gorpc"
)

// ServeHTTP 实现 http.Handler 的接口作为 RPC 请求的响应
func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// 只允许接收CONNECT请求
	if req.Method != "CONNECT" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = io.WriteString(w, "405 must CONNECT\n")
		return
	}
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Println("rpc劫持出错 ", req.RemoteAddr, ": ", err.Error())
		return
	}
	_, _ = io.WriteString(conn, "HTTP/1.0 "+CONNECTED+"\n\n")
	s.ServeConn(conn)
}

// HandleHTTP 将默认rpc地址 DefaultRpcPath 注册到 HTTP Handler 中
func (s *Server) HandleHTTP() {
	http.Handle(DefaultRpcPath, s)
	http.Handle(DefaultDebugPath, debugHTTP{s})
	log.Println("rpc server debug path: ", DefaultDebugPath)
}

// HandleHTTP 默认服务器注册HTTP的方法
func HandleHTTP() {
	DefaultServer.HandleHTTP()
}

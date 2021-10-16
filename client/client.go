package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"gorpc/codec"
	"gorpc/option"
)

// Call 承载一次 rpc 调用
type Call struct {
	Seq           uint64      // 请求编号
	ServiceMethod string      // 格式 <service>.<method>
	Args          interface{} // 参数
	Reply         interface{} // 响应
	Error         error       // 错误信息
	Done          chan *Call  // 调用完成标记
}

// done 调用结束时, 调用本方法通知调用方
func (c *Call) done() {
	c.Done <- c
}

// Client 代表一个 rpc 客户端
// 可能存在一个客户端关联多个未完成的调用
// 并且被多个goroutine运行的情况
type Client struct {
	cc       codec.Codec      // 消息编解码器, 序列化将要发送出去的请求，以及反序列化接收到的响应
	opt      *option.Option   // rpc的参数, 包含魔数和 codec.Type
	sending  sync.Mutex       // 保证请求的有序发送, 避免出现多个请求报文混淆
	header   codec.Header     // 每个请求的消息头, 只有在请求发送时才需要, 而请求发送是互斥的, 因此每个客户端只需要一个
	mu       sync.Mutex       // 全局互斥锁, 保证操作的完整性
	seq      uint64           // 给发送的请求编号, 每个请求拥有唯一编号
	pending  map[uint64]*Call // 存储未处理完的请求, 键是编号, 值是 Call 实例
	closing  bool             // 标识服务器关闭, 由用户主动关闭
	shutdown bool             // 标识服务器关闭, 一般是错误产生导致的关闭
}

// 保证Client的方法都已实现
var _ io.Closer = (*Client)(nil)

var ErrShutdown = errors.New("连接已经关闭")

// Close 关闭连接
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing {
		return ErrShutdown
	}
	c.closing = true
	return c.cc.Close()
}

// IsAvailable 当Client可用时返回true
func (c *Client) IsAvailable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.shutdown && !c.closing
}

// registerCall 注册调用, 并更新 c.seq
func (c *Client) registerCall(call *Call) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing || c.shutdown {
		return 0, ErrShutdown
	}
	call.Seq = c.seq
	c.pending[call.Seq] = call
	c.seq++
	return call.Seq, nil
}

// removeCall 根据请求编号移除某个调用并返回该调用
func (c *Client) removeCall(seq uint64) *Call {
	c.mu.Lock()
	defer c.mu.Unlock()
	call := c.pending[seq]
	delete(c.pending, seq)
	return call
}

// terminateCalls 在服务端或客户端发生错误时调用
// 将 shutdown 设置为 true, 且将错误信息通知所有 pending 状态的 call
func (c *Client) terminateCalls(err error) {
	c.sending.Lock()
	defer c.sending.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.shutdown = true
	for _, call := range c.pending {
		call.Error = err
		call.done()
	}
}

// receive 接收响应
// 对应三种情况
// call 不存在, 可能是请求没有发送完整, 或者因为其他原因被取消, 但是服务端仍旧处理了
// call 存在, 但服务端处理出错, 即 h.Error 不为空
// call 存在, 服务端处理正常, 那么需要从 body 中读取 Reply 的值
func (c *Client) receive() {
	var err error
	// 一直接收直至结束或出错
	for err == nil {
		var h codec.Header
		// 读取请求头
		if err = c.cc.ReadHeader(&h); err != nil {
			break
		}
		// 从 pending 队列中移除本次调用
		call := c.removeCall(h.Seq)
		switch {
		case call == nil:
			// call 不存在, 可能是请求没有发送完整, 或者因为其他原因被取消, 但是服务端仍旧处理了
			err = c.cc.ReadBody(nil)
		case h.Error != "":
			// call 存在, 但服务端处理出错, 即 h.Error 不为空
			call.Error = fmt.Errorf(h.Error)
			err = c.cc.ReadBody(nil)
			call.done()
		default:
			if err = c.cc.ReadBody(call.Reply); err != nil {
				call.Error = errors.New("读取消息体出错 " + err.Error())
			}
			call.done()
		}
	}
	// 发生错误, 结束 pending 队列中的剩余调用
	c.terminateCalls(err)
}

// send 发送请求
func (c *Client) send(call *Call) {
	// 确保完整发送一次请求
	c.sending.Lock()
	defer c.sending.Unlock()
	// 注册调用
	seq, err := c.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}
	// 设置请求头
	c.header.ServiceMethod = call.ServiceMethod
	c.header.Seq = seq
	c.header.Error = ""
	// 编码 & 发送请求
	if err = c.cc.Write(&c.header, call.Args); err != nil {
		call = c.removeCall(seq)
		// call 为空可能是写入部分失败
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

// Go 异步调用 rpc 服务接口, 返回 Call 实例
func (c *Client) Go(ServiceMethod string, args, reply interface{}, done chan *Call) *Call {
	switch {
	case done == nil:
		done = make(chan *Call, 1) // 带缓冲的通道
	case cap(done) == 0:
		log.Panic("rpc client: done channel 为无缓冲通道")
	}
	call := &Call{
		ServiceMethod: ServiceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	c.send(call)
	return call
}

// Call 同步调用 Go, 阻塞等待发送完成
func (c *Client) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	call := c.Go(serviceMethod, args, reply, make(chan *Call, 1))
	select {
	case <-ctx.Done():
		c.removeCall(call.Seq)
		return errors.New("rpc client: 调用失败 " + ctx.Err().Error())
	case call = <-call.Done:
		return call.Error
	}
}

// NewClient 创建 rpc 客户端实例
func NewClient(conn net.Conn, opt *option.Option) (*Client, error) {
	// 通过 opt.CodecType 获取编解码函数
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		err := fmt.Errorf("非法的codec类型 %s", opt.CodecType)
		log.Println("rpc client: codec error ", err)
		return nil, err
	}
	// 对 opt 进行编码
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("rpc client: option encode error ", err)
		_ = conn.Close()
		return nil, err
	}
	return newClientCodec(f(conn), opt), nil
}

func newClientCodec(cc codec.Codec, opt *option.Option) *Client {
	client := &Client{
		cc:      cc,
		opt:     opt,
		seq:     1, // 请求编号从1开始, 0代表非法调用
		pending: make(map[uint64]*Call),
	}
	// 开启子协程接收响应
	go client.receive()
	return client
}

// parseOptions 简化用户调用, ...*Option 将 Option 设为可选参数
func parseOptions(opts ...*option.Option) (*option.Option, error) {
	if len(opts) == 0 || opts[0] == nil {
		return option.DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, errors.New("opts 参数数量不能超过1")
	}
	opt := opts[0]
	opt.MagicNumber = option.DefaultOption.MagicNumber
	if opt.CodecType == "" {
		opt.CodecType = option.DefaultOption.CodecType
	}
	return opt, nil
}

type clientResult struct {
	client *Client
	err    error
}

type newClientFunc func(conn net.Conn, opt *option.Option) (client *Client, err error)

func dialTimout(f newClientFunc, network, address string, opts ...*option.Option) (client *Client, err error) {
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTimeout(network, address, opt.ConnectTimeout)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()
	ch := make(chan clientResult)
	go func() {
		client, err = f(conn, opt)
		ch <- clientResult{client: client, err: err}
	}()
	// 默认不设超时限制
	if opt.ConnectTimeout == 0 {
		result := <-ch
		return result.client, result.err
	}
	select {
	case <-time.After(opt.ConnectTimeout):
		return nil, fmt.Errorf("rpc client: 连接超时, 超时时间为%s", opt.ConnectTimeout)
	case result := <-ch:
		return result.client, result.err
	}
}

// Dial 通过 network 和 address 连接 rpc 服务器
func Dial(network, address string, opts ...*option.Option) (client *Client, err error) {
	return dialTimout(NewClient, network, address, opts...)
}

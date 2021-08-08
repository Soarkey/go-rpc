package codec

import "io"

// Header 请求头信息
type Header struct {
	ServiceMethod string // 格式: 服务名.方法名
	Seq           uint64 // 请求的序号
	Error         string // 错误信息
}

// Codec 对消息体进行编解码的接口
type Codec interface {
	io.Closer
	ReadHeader(header *Header) error
	ReadBody(body interface{}) error
	Write(head *Header, body interface{}) error
}

// NewCodecFunc 新建Codec对象的函数类型
type NewCodecFunc func(closer io.ReadWriteCloser) Codec

// Type 序列化/反序列化类型
type Type string

const (
	// GobType Gob (Go binary) 是 Go 自己的以二进制形式序列化和反序列化程序数据的格式
	// 特定地用于纯 Go 的环境中
	// 类似于 Python 的 pickle 和 Java 的 Serialization
	GobType  Type = "application/gob"
	JsonType Type = "application/json"
)

// NewCodecFuncMap 通过Type可以拿到构建新建Codec函数的map
var NewCodecFuncMap map[Type]NewCodecFunc

// init 导入包即执行
func init() {
	// 此部分类似工厂设计模式, 客户端和服务端通过Codec的Type得到构造函数, 从而建立Codec实例
	// 不同的是返回的是构造函数, 而不是实例对象
	NewCodecFuncMap = make(map[Type]NewCodecFunc)
	NewCodecFuncMap[GobType] = NewGobCodec
	// json版本暂未实现
	// NewCodecFuncMap[JsonType] = JsonType
}

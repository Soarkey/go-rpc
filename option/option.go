package option

import (
	"time"

	"gorpc/codec"
)

// MagicNumber 魔数
const MagicNumber = 0x3bef5c

type Option struct {
	MagicNumber    int           // 魔数, 标记这是一个go-rpc自定义请求
	CodecType      codec.Type    // client端可以选择不同的Codec来编码body
	ConnectTimeout time.Duration // 0 表示不设限
	HandleTimeout  time.Duration
}

var DefaultOption = &Option{
	MagicNumber:    MagicNumber,
	CodecType:      codec.GobType,
	ConnectTimeout: 10 * time.Second, // 默认超时时间为10s
}

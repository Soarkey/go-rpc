package codec

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

// GobCodec
// 结构体由四部分构成：
// conn 是由构建函数传入, 通常是通过 TCP 或者 Unix 建立 socket 时得到的连接实例
// dec 和 enc 对应 gob 的 Decoder 和 Encoder
// buf 是为了防止阻塞而创建的带缓冲的 Writer, 一般这么做能提升性能
type GobCodec struct {
	conn io.ReadWriteCloser
	buf  *bufio.Writer
	dec  *gob.Decoder
	enc  *gob.Encoder
}

// ReadHeader 读取请求头
func (c GobCodec) ReadHeader(header *Header) error {
	return c.dec.Decode(header)
}

// ReadBody 读取请求体
func (c GobCodec) ReadBody(body interface{}) error {
	return c.dec.Decode(body)
}

// Write 将header和body编码, 写入缓冲区后刷新并关闭
func (c GobCodec) Write(header *Header, body interface{}) (err error) {
	defer func() {
		_ = c.buf.Flush()
		if err != nil {
			_ = c.Close()
		}
	}()
	if err = c.enc.Encode(header); err != nil {
		log.Fatalln("rpc codec: gob编码header出错 err: ", err)
		return err
	}
	if err = c.enc.Encode(body); err != nil {
		log.Fatalln("rpc codec: gob编码body出错 err: ", err)
		return err
	}
	return nil
}

// Close 关闭连接
func (c GobCodec) Close() error {
	return c.conn.Close()
}

// 利用强制类型转换, 确保 GobCodec 接口的所有方法已实现
var _ Codec = (*GobCodec)(nil)

// NewGobCodec 构建 GobCodec 对象
func NewGobCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn: conn,
		buf:  buf,
		dec:  gob.NewDecoder(conn),
		enc:  gob.NewEncoder(buf),
	}
}

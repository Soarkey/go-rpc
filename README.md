# go-rpc

用go实现rpc, 参考 https://github.com/geektutu/7days-golang

## 1.消息编解码+服务端

- 使用 encoding/gob 实现消息编解码 (序列化与反序列化)
- 实现一个简易的服务端，仅接受消息，不处理

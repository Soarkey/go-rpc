# go-rpc

用go实现rpc, 参考 https://github.com/geektutu/7days-golang

## 1.消息编解码+服务端

- 使用 encoding/gob 实现消息编解码 (序列化与反序列化)
- 实现一个简易的服务端，仅接受消息，不处理

## 2.高性能客户端

- 定义一次rpc调用的承载结构体Call
- 实现客户端Call同步调用方法和Go异步调用方法

## 3.服务注册

- 利用反射构建Service的注册
- 集成到服务端实现服务的注册和调用
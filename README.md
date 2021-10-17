# go-rpc

用go实现rpc, 参考 https://github.com/geektutu/7days-golang

## 1.消息编解码+服务端

- 使用`encoding/gob`实现消息编解码 (序列化与反序列化)
- 实现一个简易的服务端，仅接受消息，不处理

## 2.高性能客户端

- 定义一次rpc调用的承载结构体Call
- 实现客户端Call同步调用方法和Go异步调用方法

## 3.服务注册

- 利用反射构建Service的注册
- 集成到服务端实现服务的注册和调用

## 4.超时处理

- 设定连接超时和处理超时两个字段
- 利用`select + channel + context.WithTimeout`实现超时处理

## 5.支持HTTP协议

- 使用HTTP协议的CONNECT方法做代理, 通过`http.Hijacker`劫持HTTP请求
- Debug页面展示所有已注册的服务调用统计视图

## 6.负载均衡

- 客户端支持随机/轮询负载均衡
- 手动注册服务实例
- (存在发送请求偶尔出现死锁的bug)

## 7.服务发现与注册中心

- 注册中心架构图
    ```
         [ registry ]
         ↑|        ↑
     pull||push    |register/heartbeat
         |↓        |
    [client] ---> [server]
             call
    ```
- 实现通过`GET/POST+Header参数`支持注册和心跳保活的注册中心
- 客户端定时从注册中心获取最新服务列表
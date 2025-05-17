package main

import (
	"context"
	"log"

	"thrift-client/gen-go/hello" // 引入 Thrift 自动生成的客户端接口定义

	"github.com/apache/thrift/lib/go/thrift" // 引入 Thrift Go 库
)

func main() {
	// 创建一个到服务端的 TCP 连接（对应服务端监听地址）
	transport, err := thrift.NewTSocket("localhost:9091")
	if err != nil {
		log.Fatalf("Error opening socket: %v", err)
	}

	// 创建默认的二进制协议工厂（可选用 JSON、Compact 等）
	protocolFactory := thrift.NewTBinaryProtocolFactoryDefault()

	// 创建客户端对象，绑定传输层和协议
	client := hello.NewHelloServiceClientFactory(transport, protocolFactory)

	// 打开连接，准备与服务端通信
	if err := transport.Open(); err != nil {
		log.Fatalf("Error opening transport: %v", err)
	}
	defer transport.Close()

	// 调用远程方法 SayHello，发送参数 "Thrift"
	resp, err := client.SayHello(context.Background(), "Thrift")
	if err != nil {
		log.Fatalf("Error calling SayHello: %v", err)
	}

	// 打印从服务端返回的结果
	log.Println("Response from server:", resp)
}

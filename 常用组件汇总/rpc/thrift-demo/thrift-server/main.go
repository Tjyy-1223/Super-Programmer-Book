package main

import (
	"log"
	"thrift-server/gen-go/hello" // 引入 Thrift 生成的服务接口定义
	"thrift-server/handler"      // 引入自定义的服务实现

	"github.com/apache/thrift/lib/go/thrift" // 引入 Thrift 的 Go 库
)

func main() {
	addr := "localhost:9091" // 服务监听地址和端口

	// 创建处理器，绑定服务实现逻辑（handler.HelloHandler 实现了 HelloService 接口）
	processor := hello.NewHelloServiceProcessor(&handler.HelloHandler{})

	// 创建一个基于 TCP 的服务端 socket，用于监听客户端请求
	transport, err := thrift.NewTServerSocket(addr)
	if err != nil {
		log.Fatalf("Error opening socket: %v", err)
	}

	// 创建 Thrift 服务端对象，使用 TSimpleServer2 实现
	server := thrift.NewTSimpleServer2(processor, transport)

	log.Println("Starting Thrift server on", addr)
	// 启动服务端，开始监听并处理客户端请求
	if err := server.Serve(); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}

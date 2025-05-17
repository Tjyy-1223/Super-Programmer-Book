package main

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"              // 导入 gRPC 框架库
	pb "grpc-client/grpc-demo/helloworld" // 导入根据 proto 文件生成的 gRPC 客户端代码
)

func main() {
	// 建立与 gRPC 服务端的连接（连接到 localhost:50051）
	// grpc.WithInsecure(): 允许使用不加密的连接（仅用于开发测试）
	// grpc.WithBlock(): 阻塞直到连接成功（否则是异步连接）
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("Did not connect: %v", err)
	}
	defer conn.Close() // 程序结束时关闭连接

	// 创建一个 gRPC 客户端实例
	client := pb.NewGreeterClient(conn)

	// 创建上下文，设置 1 秒超时，防止调用卡死
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// 调用远程 SayHello 方法，并传入 HelloRequest 请求参数
	resp, err := client.SayHello(ctx, &pb.HelloRequest{Name: "World"})
	if err != nil {
		log.Fatalf("Could not greet: %v", err)
	}

	// 输出服务端返回的响应信息
	log.Printf("Greeting: %s", resp.Message)
}

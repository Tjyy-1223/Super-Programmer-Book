package main

import (
	"context"
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"              // 导入 gRPC 框架库
	pb "grpc-server/grpc-demo/helloworld" // 导入生成的 gRPC 协议代码包（根据 proto 文件生成）
)

// 定义服务结构体，嵌入生成的默认实现结构体（用于版本兼容）
type greeterServer struct {
	pb.UnimplementedGreeterServer
}

// SayHello 实现 SayHello 方法（对应 proto 中的 rpc 方法）
// 接收一个 HelloRequest，返回一个 HelloReply
func (s *greeterServer) SayHello(ctx context.Context, req *pb.HelloRequest) (*pb.HelloReply, error) {
	reply := fmt.Sprintf("Hello, %s", req.Name) // 拼接问候语
	return &pb.HelloReply{Message: reply}, nil  // 返回响应
}

func main() {
	// 启动 TCP 监听，绑定 50051 端口
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// 创建 gRPC 服务器实例
	s := grpc.NewServer()

	// 将我们实现的 greeterServer 注册到 gRPC 服务器
	pb.RegisterGreeterServer(s, &greeterServer{})

	log.Println("gRPC server listening on :50051")

	// 启动服务器，开始阻塞式监听来自客户端的请求
	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

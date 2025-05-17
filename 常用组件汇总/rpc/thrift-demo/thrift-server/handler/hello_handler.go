package handler

import (
	"context"
)

// HelloHandler 是 HelloService 接口的实现者，用于处理客户端发来的请求。
type HelloHandler struct{}

// SayHello 实现了 hello.HelloService 接口中的 SayHello 方法。
// 接收一个字符串 name，返回一个问候语 "Hello, name"。
func (h *HelloHandler) SayHello(ctx context.Context, name string) (string, error) {
	return "Hello, " + name, nil
}

package main

import (
	"jdlogin/opencv/server"
	pb "jdlogin/proto"
	"log"
	"net"

	"google.golang.org/grpc"
)

func main() {
	// 创建 Tcp 连接
	listener, err := net.Listen("tcp", "127.0.0.1:12221")
	if err != nil {
		log.Fatalf("监听失败: %v", err)
	}
	// 创建gRPC服务
	grpcServer := grpc.NewServer()

	pb.RegisterOpenCVServer(grpcServer, &server.OpenCVServer{})

	err = grpcServer.Serve(listener)
	if err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

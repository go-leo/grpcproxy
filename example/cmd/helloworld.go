package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-leo/grpcproxy"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	"github.com/go-leo/grpcproxy/example/api/helloworld"
)

/*
启动服务
go run cmd/helloworld.go

执行测试请求：
curl http://127.0.0.1:8088/v1/SayHello?name=leo
输出：
{"code":0,"msg":"","data":{"message":"Hello leo"}}

curl --request POST 'http://127.0.0.1:8088/v2/p' --data-raw '{"name":"101"}'
输出：
{"code":0,"msg":"","data":{"message":"Hello 101"}}%
*/
func main() {
	startGrpcServer()
	// Set up a connection to the server.
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	engine := gin.New()
	c := helloworld.NewGreeterClient(conn)
	routes := helloworld.GreeterProxyRoutes(c)
	engine = grpcproxy.AppendRoutes(engine, routes...)

	srv := http.Server{Handler: engine}
	listen, err := net.Listen("tcp", ":8088")
	if err != nil {
		panic(err)
	}
	err = srv.Serve(listen)
	if err != nil {
		panic(err)
	}
}

// 启动grpc服务，用gin代理进来到groc
func startGrpcServer() {
	// 启动gRPC服务器
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	helloworld.RegisterGreeterServer(s, &ApiServer{})
	reflection.Register(s)
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
}

type ApiServer struct {
	helloworld.UnimplementedGreeterServer
}

func (s *ApiServer) SayHello(ctx context.Context, in *helloworld.HelloRequest) (*helloworld.HelloReply, error) {
	fmt.Println("SayHello")
	return &helloworld.HelloReply{Message: "Hello " + in.Name}, nil
}

func (ApiServer) PostV2(ctx context.Context, in *helloworld.HelloRequest) (*helloworld.HelloReply, error) {
	fmt.Println("PostV2")
	return &helloworld.HelloReply{Message: "Hello " + in.Name}, nil
}

func (ApiServer) mustEmbedUnimplementedGreeterServer() {}

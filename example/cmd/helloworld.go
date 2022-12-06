package main

import (
	"log"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/go-leo/grpcproxy"
	"github.com/go-leo/grpcproxy/example/api/helloworld"
)

func main() {

	// Set up a connection to the server.
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := helloworld.NewGreeterClient(conn)

	var routes []grpcproxy.Route
	routes = append(routes, helloworld.GreeterProxyRoutes(c)...)

	engine := gin.New()
	for _, route := range routes {
		engine.Handle(route.Method(), route.Path(), route.Handler())
	}

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

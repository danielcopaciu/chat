package main

import (
	"github.com/danielcopaciu/chat/generated/chat"
	"golang.org/x/crypto/acme/autocert"

	"github.com/cloudflare/cfssl/log"
	"google.golang.org/grpc"
)

func startGRPCServer(address string, server chat.ChatServer) (func(), error) {
	// lis, err := net.Listen("tcp", address)
	// if err != nil {
	// 	return nil, err
	// }

	lis := autocert.NewListener("chat.dragffy.ro")

	grpcServer := grpc.NewServer()
	chat.RegisterChatServer(grpcServer, server)

	log.Infof("Starting GRPC server on: %s", address)
	go grpcServer.Serve(lis)

	return func() {
		log.Info("Stopping GRPC server")
		grpcServer.GracefulStop()
	}, nil
}

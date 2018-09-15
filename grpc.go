package main

import (
	"net"

	"github.com/danielcopaciu/chat/generated/chat"

	"github.com/cloudflare/cfssl/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func startGRPCServer(address string, creds credentials.TransportCredentials, server chat.ChatServer) (func(), error) {
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	grpcServer := grpc.NewServer(grpc.Creds(creds))
	chat.RegisterChatServer(grpcServer, server)

	log.Infof("Starting GRPC server on: %s", address)
	go grpcServer.Serve(lis)

	return func() {
		log.Info("Stopping GRPC server")
		grpcServer.GracefulStop()
	}, nil
}

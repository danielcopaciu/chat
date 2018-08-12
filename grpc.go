package main

import (
	"crypto/tls"
	"net"
	"net/http"

	"github.com/danielcopaciu/chat/generated/chat"
	"golang.org/x/crypto/acme/autocert"

	"github.com/cloudflare/cfssl/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func startGRPCServer(address string, server chat.ChatServer) (func(), error) {
	m := &autocert.Manager{
		Cache:      autocert.DirCache("tls"),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist("chat.dragffy.ro"),
	}
	go http.ListenAndServe(":http", m.HTTPHandler(nil))
	creds := credentials.NewTLS(&tls.Config{GetCertificate: m.GetCertificate})

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

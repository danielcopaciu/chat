package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"

	"google.golang.org/grpc/codes"

	"google.golang.org/grpc/status"

	"github.com/daniel.copaciu/chat/generated/chat"
	"google.golang.org/grpc/metadata"
)

type Server struct {
	clients   map[string]chan chat.Message
	messages  chan chat.Message
	clientMtx sync.Mutex
}

func NewServer() *Server {
	return &Server{
		clients:  make(map[string]chan chat.Message),
		messages: make(chan chat.Message, 1000),
	}
}

func (s *Server) Login(ctx context.Context, req *chat.LoginRequest) (*chat.LoginResponse, error) {
	s.clientMtx.Lock()
	s.clients[req.Username] = make(chan chat.Message, 100)
	s.clientMtx.Unlock()

	message := fmt.Sprintf("%s has joined the conversation", req.Username)
	s.messages <- chat.Message{Value: message}

	return &chat.LoginResponse{}, nil
}

func (s *Server) Logout(ctx context.Context, req *chat.LogoutRequest) (*chat.LogoutResponse, error) {
	s.clientMtx.Lock()
	delete(s.clients, req.Username)
	s.clientMtx.Unlock()

	message := fmt.Sprintf("%s has left the conversation", req.Username)
	s.messages <- chat.Message{Value: message}

	return &chat.LogoutResponse{}, nil
}

func (s *Server) Join(stream chat.Chat_JoinServer) error {
	metadata, ok := metadata.FromIncomingContext(stream.Context())
	if !ok {
		return status.Error(codes.Internal, "Failed to read metadata")
	}

	data := metadata["username"]
	if len(data) == 0 {
		return status.Error(codes.Internal, "Unknown user")
	}

	username := data[0]
	s.clientMtx.Lock()
	client := s.clients[username]
	s.clientMtx.Unlock()

	if client == nil {
		return status.Error(codes.Unauthenticated, "Unauthenticated user")
	}

	go func(){
		defer close(client)
		s.sendMessage(stream, client)
	}()

	for {
		req, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		s.messages <- *req
	}

	<-stream.Context().Done()
	return stream.Context().Err()
}

func (s *Server) sendMessage(stream chat.Chat_JoinServer, client chan chat.Message) {
	for res := range client {
		err := stream.Send(&res)
		if status, ok := status.FromError(err); ok {
			switch status.Code() {
			case codes.OK:
			case codes.Unavailable, codes.Canceled, codes.DeadlineExceeded:
				log.Print("Client connection terminated")
				return
			}
		}
	}
}

func (s *Server) Run(ctx context.Context) {
	defer close(s.messages)
	
	for {
		select {
		case <-ctx.Done():
			return
		case response := <-s.messages:
			for _, client := range s.clients {
				client <- response
			}
		}
	}
}

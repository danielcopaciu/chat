package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/golang/protobuf/proto"

	"google.golang.org/grpc/codes"

	"google.golang.org/grpc/status"

	"github.com/danielcopaciu/chat/generated/chat"
	"github.com/pkg/errors"
	"google.golang.org/grpc/metadata"
)

var hash = sha256.New()

type Server struct {
	clients       map[string]*Session
	messages      chan chat.Envelope
	clientMtx     sync.Mutex
	encryptionKey *rsa.PrivateKey
}

type Session struct {
	messageBus chan chat.Envelope
	clientKey  *rsa.PublicKey
}

func NewServer() (*Server, error) {
	encryptionKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate server key")
	}

	return &Server{
		clients:       make(map[string]*Session),
		messages:      make(chan chat.Envelope, 1000),
		encryptionKey: encryptionKey,
	}, nil
}

func (s *Server) Login(ctx context.Context, req *chat.LoginRequest) (*chat.LoginResponse, error) {
	s.clientMtx.Lock()
	defer s.clientMtx.Unlock()

	session := &Session{
		messageBus: make(chan chat.Envelope, 100),
	}

	block, _ := pem.Decode(req.ClientKey)
	if block == nil {
		return nil, errors.New("invalid key received from client")
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, errors.New("invalid key received from client")
	}

	switch pubKey.(type) {
	case *rsa.PublicKey:
		session.clientKey = pubKey.(*rsa.PublicKey)
	default:
		return nil, errors.New("client key has an invalid type")
	}

	s.clients[req.Username] = session

	message, err := proto.Marshal(&chat.Message{Value: fmt.Sprintf("%s has joined the conversation", req.Username)})
	if err != nil {
		return nil, err
	}

	encrpted, err := rsa.EncryptOAEP(hash, rand.Reader, &s.encryptionKey.PublicKey, message, nil)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to encrypt message")
	}

	s.messages <- chat.Envelope{Message: encrpted}

	publicKey, err := x509.MarshalPKIXPublicKey(&s.encryptionKey.PublicKey)
	if err != nil {
		return &chat.LoginResponse{}, status.Error(codes.Internal, "failed to create session for client")
	}

	pubBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: publicKey,
	})

	return &chat.LoginResponse{
		ServerKey: pubBytes,
	}, nil
}

func (s *Server) Logout(ctx context.Context, req *chat.LogoutRequest) (*chat.LogoutResponse, error) {
	s.clientMtx.Lock()
	delete(s.clients, req.Username)
	s.clientMtx.Unlock()

	message, err := proto.Marshal(&chat.Message{Value: fmt.Sprintf("%s has left the conversation", req.Username)})
	if err != nil {
		return nil, err
	}

	encrpted, err := rsa.EncryptOAEP(hash, rand.Reader, &s.encryptionKey.PublicKey, message, nil)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to encrypt message")
	}

	s.messages <- chat.Envelope{Message: encrpted}
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
	session := s.clients[username]
	s.clientMtx.Unlock()

	if session == nil {
		return status.Error(codes.Unauthenticated, "Unauthenticated user")
	}

	go func() {
		defer close(session.messageBus)
		s.sendMessage(stream, session)
	}()

	for {
		env, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		s.messages <- *env
	}

	<-stream.Context().Done()
	return stream.Context().Err()
}

func (s *Server) sendMessage(stream chat.Chat_JoinServer, session *Session) error {
	for env := range session.messageBus {
		decrypted, err := rsa.DecryptOAEP(hash, rand.Reader, s.encryptionKey, env.Message, nil)
		if err != nil {
			return errors.WithMessage(err, "failed to decrypt message")
		}

		enrypted, err := rsa.EncryptOAEP(hash, rand.Reader, session.clientKey, decrypted, nil)
		if err != nil {
			return errors.WithMessage(err, "failed to encrypt message")
		}

		err = stream.Send(&chat.Envelope{Message: enrypted})
		if status, ok := status.FromError(err); ok {
			switch status.Code() {
			case codes.OK:
			case codes.Unavailable, codes.Canceled, codes.DeadlineExceeded:
				log.Print("Client connection terminated")
				return nil
			}
		}
	}
	return nil
}

func (s *Server) Run(ctx context.Context) {
	defer close(s.messages)
	for {
		select {
		case <-ctx.Done():
			return
		case env := <-s.messages:
			for _, session := range s.clients {
				session.messageBus <- env
			}
		}
	}
}

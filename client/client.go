package client

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/gogo/protobuf/proto"

	"github.com/danielcopaciu/chat/generated/chat"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

var hash = sha256.New()

type Client struct {
	username        string
	serverAddress   string
	insecure        bool
	chatClient      chat.ChatClient
	privateKey      *rsa.PrivateKey
	publicServerKey *rsa.PublicKey
}

func NewClient(username, serverAddress string, insecure bool) (*Client, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to generate key")
	}

	return &Client{
		username:      username,
		serverAddress: serverAddress,
		insecure:      insecure,
		privateKey:    privateKey,
	}, nil
}

func (c *Client) Login(ctx context.Context) error {
	loginCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	fmt.Printf("%T", c.privateKey.PublicKey)
	publicKey, err := x509.MarshalPKIXPublicKey(&c.privateKey.PublicKey)
	if err != nil {
		return errors.WithMessage(err, "failed to generate client key")
	}

	pubBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: publicKey,
	})

	loginResponse, err := c.chatClient.Login(loginCtx, &chat.LoginRequest{Username: c.username, ClientKey: pubBytes})
	if err != nil {
		return err
	}

	block, _ := pem.Decode(loginResponse.ServerKey)
	if block == nil {
		return errors.New("invalid key received from server")
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return errors.New("invalid key received from server")
	}

	switch pubKey.(type) {
	case *rsa.PublicKey:
		c.publicServerKey = pubKey.(*rsa.PublicKey)
	default:
		return errors.New("server key has an invalid type")
	}
	return nil
}

func (c *Client) Run(ctx context.Context) error {
	connCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var creds grpc.DialOption
	if c.insecure {
		creds = grpc.WithInsecure()
	} else {
		creds = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{}))
	}
	conn, err := grpc.DialContext(connCtx, c.serverAddress,
		creds,
		grpc.WithWaitForHandshake(),
		grpc.WithBlock(),
	)
	if err != nil {
		return errors.WithMessage(err, "unable to connect")
	}
	defer conn.Close()
	log.Printf("Succesfully connected to server on %s\n", c.serverAddress)

	c.chatClient = chat.NewChatClient(conn)
	if err := c.Login(ctx); err != nil {
		return err
	}

	md := metadata.New(map[string]string{"username": c.username})
	clientContext := metadata.NewOutgoingContext(ctx, md)

	clientContext, cancel = context.WithCancel(clientContext)
	defer cancel()

	stream, err := c.chatClient.Join(clientContext)
	defer stream.CloseSend()

	sendErrs := make(chan error)
	go func() {
		defer close(sendErrs)
		sendErrs <- c.send(stream)
	}()

	receiveErrs := make(chan error)
	go func() {
		defer close(receiveErrs)
		receiveErrs <- c.receive(stream)
	}()

	select {
	case err := <-sendErrs:
		return err
	case err := <-receiveErrs:
		return err
	case <-ctx.Done():
		return nil
	}
}

func (c *Client) Logout() error {
	logoutRequest := &chat.LogoutRequest{
		Username: c.username,
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := c.chatClient.Logout(ctx, logoutRequest)
	return err
}

func (c *Client) getEnvelope(msg chat.Message) (*chat.Envelope, error) {
	data, err := proto.Marshal(&msg)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to send message")
	}

	encrypted, err := rsa.EncryptOAEP(hash, rand.Reader, c.publicServerKey, data, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to send message")
	}

	return &chat.Envelope{Message: encrypted}, nil
}

func (c *Client) send(stream chat.Chat_JoinClient) error {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		value := scanner.Text()
		switch value {
		case "/logout":
			return c.Logout()
		default:
			message := chat.Message{
				Sender: c.username,
				Value:  scanner.Text(),
			}
			env, err := c.getEnvelope(message)
			if err != nil {
				return err
			}

			if err := stream.Send(env); err != nil {
				return err
			}
		}
	}

	return scanner.Err()
}

func (c *Client) receive(stream chat.Chat_JoinClient) error {
	for {
		env, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		decrypted, err := rsa.DecryptOAEP(hash, rand.Reader, c.privateKey, env.Message, nil)
		if err != nil {
			return errors.WithMessage(err, "failed to read message")
		}

		var msg chat.Message
		err = proto.Unmarshal(decrypted, &msg)
		if err != nil {
			return errors.WithMessage(err, "failed to read message")
		}

		if msg.Sender != "" {
			fmt.Printf("%s: %s\n", msg.Sender, msg.Value)
		} else {
			fmt.Printf("%s\n", msg.Value)
		}
	}
}

package client

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/daniel.copaciu/chat/generated/chat"

	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type Client struct {
	username      string
	serverAddress string
	chatClient    chat.ChatClient
}

func NewClient(username, serverAddress string) *Client {
	return &Client{
		username:      username,
		serverAddress: serverAddress,
	}
}

func (c *Client) Login(ctx context.Context) error {
	loginCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := c.chatClient.Login(loginCtx, &chat.LoginRequest{Username: c.username})
	return err
}

func (c *Client) Run(ctx context.Context) error {
	connCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	log.Printf("Connection to server on %s\n", c.serverAddress)

	conn, err := grpc.DialContext(connCtx, c.serverAddress, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return errors.WithMessage(err, "unable to connect")
	}
	defer conn.Close()

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

func (c *Client) send(stream chat.Chat_JoinClient) error {
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		value := scanner.Text()
		switch value {
		case "/logout":
			return c.Logout()
		default:
			message := &chat.Message{
				Sender: c.username,
				Value:  scanner.Text(),
			}

			if err := stream.Send(message); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) receive(stream chat.Chat_JoinClient) error {
	for {
		res, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		if res.Sender != "" {
			fmt.Printf("%s: %s\n", res.Sender, res.Value)
		} else {
			fmt.Printf("%s\n", res.Value)
		}
	}
}

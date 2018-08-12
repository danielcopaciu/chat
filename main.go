package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/daniel.copaciu/chat/client"

	"github.com/daniel.copaciu/chat/server"

	cli "github.com/jawher/mow.cli"
)

var (
	defaultAddress = "localhost"
	defaultPort    = "8090"
	appMeta        = struct {
		name        string
		description string
	}{
		name:        "chat",
		description: "Chat application",
	}
)

func main() {
	app := cli.App(appMeta.name, appMeta.description)

	app.Command("server", "Run server chat", func(cmd *cli.Cmd) {
		address := app.String(cli.StringOpt{
			Name:   "address",
			Value:  fmt.Sprintf("%s:%s", defaultAddress, defaultPort),
			Desc:   "GRPC address",
			EnvVar: "ADDRESS",
		})

		cmd.Action = func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if err := runServer(ctx, *address); err != nil {
				cancel()
				log.Fatal(err)
			}
		}
	})

	app.Command("client", "Run server client", func(cmd *cli.Cmd) {
		serverAddress := app.String(cli.StringOpt{
			Name:  "serverAddress",
			Value: "localhost:8090",
			Desc:  "Address of the chat server",
		})

		cmd.Action = func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if err := runClient(ctx, *serverAddress); err != nil {
				log.Fatal(err)
			}
		}
	})

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func runServer(ctx context.Context, address string) error {
	chatServer := server.NewServer()
	serverStop, err := startGRPCServer(address, chatServer)
	if err != nil {
		return err
	}
	defer serverStop()

	serverContext, cancel := context.WithCancel(context.Background())
	go func() {
		chatServer.Run(serverContext)
	}()

	exit := make(chan os.Signal)
	signal.Notify(exit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
		cancel()
	case <-exit:
		cancel()
	}

	return nil
}

func runClient(ctx context.Context, serverAddress string) error {
	fmt.Print("Username: ")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()

	username := scanner.Text()
	client := client.NewClient(username, serverAddress)

	clientCtx, cancel := context.WithCancel(context.Background())
	errs := make(chan error)
	go func() {
		defer close(errs)
		defer cancel()
		errs <- client.Run(clientCtx)
	}()

	exit := make(chan os.Signal)
	signal.Notify(exit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-exit:
		return client.Logout()
	case err := <-errs:
		return err
	case <-ctx.Done():
		return nil
	}
}

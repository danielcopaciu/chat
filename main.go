package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/danielcopaciu/chat/client"
	"golang.org/x/crypto/acme/autocert"
	"google.golang.org/grpc/credentials"

	"github.com/danielcopaciu/chat/server"

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
		insecure := app.Bool(cli.BoolOpt{
			Name:   "insecure",
			Value:  true,
			Desc:   "Flag to run server without tls",
			EnvVar: "INSECURE",
		})
		certDir := app.String(cli.StringOpt{
			Name:   "cert-dir",
			Value:  "tls",
			Desc:   "Directory to cache acme certs (effective if insecure is false)",
			EnvVar: "CERT_DIR",
		})
		domain := app.String(cli.StringOpt{
			Name:   "domain",
			Value:  "chat.dragffy.ro",
			Desc:   "Domain name to register cert with (effective if insecure is false)",
			EnvVar: "DOMAIN",
		})

		cmd.Action = func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var creds credentials.TransportCredentials
			if !*insecure {
				m := &autocert.Manager{
					Cache:      autocert.DirCache(*certDir),
					Prompt:     autocert.AcceptTOS,
					HostPolicy: autocert.HostWhitelist(*domain),
				}
				go func() {
					log.Println("autocert manager server terminated. err:", http.ListenAndServe(":http", m.HTTPHandler(nil)))
				}()
				creds = credentials.NewTLS(&tls.Config{GetCertificate: m.GetCertificate})
			}

			if err := runServer(ctx, *address, creds); err != nil {
				cancel()
				log.Fatal(err)
			}
		}
	})

	app.Command("client", "Run server client", func(cmd *cli.Cmd) {
		serverAddress := app.String(cli.StringOpt{
			Name:   "serverAddress",
			Value:  "localhost:8090",
			Desc:   "Address of the chat server",
			EnvVar: "SERVER_ADDRESS",
		})
		insecure := app.Bool(cli.BoolOpt{
			Name:   "insecure",
			Value:  true,
			Desc:   "Flag to establish non-secure conn",
			EnvVar: "INSECURE",
		})

		cmd.Action = func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if err := runClient(ctx, *serverAddress, *insecure); err != nil {
				log.Fatal(err)
			}
		}
	})

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func runServer(ctx context.Context, address string, creds credentials.TransportCredentials) error {

	chatServer, err := server.NewServer()
	if err != nil {
		return err
	}
	serverStop, err := startGRPCServer(address, creds, chatServer)
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

func runClient(ctx context.Context, serverAddress string, insecure bool) error {
	fmt.Print("Username: ")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()

	username := scanner.Text()
	client, err := client.NewClient(username, serverAddress, insecure)
	if err != nil {
		return err
	}

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

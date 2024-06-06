package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	isServer = flag.Bool("server", false, "whether it should be run as a server")
	port     = flag.Uint("port", 1337, "port to send to or receive from")
	host     = flag.String("host", "127.0.0.1", "address to send to or receive from")
	timeout  = flag.Duration("timeout", 15*time.Second, "read and write blocking deadlines")
	input    = flag.String("input", "-", "file with contents to send over udp")
)

const maxBufferSize = 1024

func main() {
	flag.Parse()

	var (
		err     error
		address = fmt.Sprintf("%s:%d", *host, *port)
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Gracefully handle signals so that we can finalize any of our
	// blocking operations by cancelling their contexts.
	go func() {
		sigChan := make(chan os.Signal)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		cancel()
	}()

	if *isServer {
		fmt.Println("running as a server on " + address)
		err = server(ctx, address)
		if err != nil && err != context.Canceled {
			panic(err)
		}
		return
	}

	// allow the client to receive the contents to be sent
	// either via `stdin` or via a file that can be supplied
	// via the `-input=` flag.
	reader := os.Stdin
	if *input != "-" {
		file, err := os.Open(*input)
		if err != nil {
			panic(err)
		}
		defer file.Close()
		reader = file
	}

	fmt.Println("sending to " + address)
	err = client(ctx, address, reader)
	if err != nil && err != context.Canceled {
		panic(err)
	}
}

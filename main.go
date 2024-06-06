package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
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

func client(ctx context.Context, address string, reader io.Reader) (err error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return err
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return
	}

	defer conn.Close()

	doneChan := make(chan error, 1)

	go func() {
		n, err := io.Copy(conn, reader)
		if err != nil {
			doneChan <- err
			return
		}

		fmt.Printf("packet-written: bytes=%d\n", n)

		buffer := make([]byte, maxBufferSize)

		deadline := time.Now().Add(*timeout)
		err = conn.SetReadDeadline(deadline)
		if err != nil {
			doneChan <- err
			return
		}

		nRead, addr, err := conn.ReadFrom(buffer)
		if err != nil {
			doneChan <- err
			return
		}

		fmt.Printf("packet-received: bytes=%d from=%s\n", nRead, addr.String())

		doneChan <- nil
	}()

	select {
	case <-ctx.Done():
		fmt.Println("cancelled")
	case err = <-doneChan:
	}

	return
}

func server(ctx context.Context, address string) (err error) {
	pc, err := net.ListenPacket("udp", address)
	if err != nil {
		return
	}

	defer pc.Close()

	doneChan := make(chan error, 1)
	buffer := make([]byte, maxBufferSize)

	go func() {
		for {
			n, addr, err := pc.ReadFrom(buffer)
			if err != nil {
				doneChan <- err
				return
			}

			fmt.Printf("packet-received: bytes=%d from=%s\n",
				n, addr.String())

			deadline := time.Now().Add(*timeout)
			err = pc.SetWriteDeadline(deadline)
			if err != nil {
				doneChan <- err
				return
			}

			// Write the packet's contents back to the client.
			n, err = pc.WriteTo(buffer[:n], addr)
			if err != nil {
				doneChan <- err
				return
			}

			fmt.Printf("packet-written: bytes=%d to=%s\n", n, addr.String())
		}
	}()

	select {
	case <-ctx.Done():
		fmt.Println("cancelled")
		err = ctx.Err()
	case err = <-doneChan:
	}

	return
}

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

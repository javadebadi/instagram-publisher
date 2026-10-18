// Command instagram-publisher runs the Instagram publisher service: POST
// /publish/carousel, /publish/reel and /publish/story post from media URLs,
// POST /token/refresh keeps the account's long-lived token fresh. See package
// publisher.
//
// Which secret stores a token can be read from is decided here, by what this
// file registers: env: and envfile: always, gcp: (Google Secret Manager) as
// below. To run on another cloud, copy this file, register that cloud's store
// instead, and build it: the binary then contains only that one.
//
// It has no authentication of its own: run it where something else decides
// who may call it (Cloud Run's IAM).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/javadebadi/instagram-publisher/publisher"
	"github.com/javadebadi/instagram-publisher/secret"
	"github.com/javadebadi/instagram-publisher/secret/gcp"
)

// version is set at release time (-ldflags "-X main.version=v1.2.3").
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "instagram-publisher:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version") {
		fmt.Println("instagram-publisher", version)
		return nil
	}
	// "serve" is the only command; it is accepted (and optional).
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}
	fs := flag.NewFlagSet("instagram-publisher", flag.ContinueOnError)
	addr := fs.String("addr", "", "address to listen on (default :$PORT, or :8080)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *addr == "" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		*addr = ":" + port
	}
	srv := &http.Server{
		Addr:              *addr,
		Handler:           publisher.New(publisher.Config{Secrets: secrets()}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	fmt.Printf("listening on %s\n", *addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// secrets is the set of places this build can read a token from.
func secrets() *secret.Resolver {
	r := secret.NewResolver() // env:, envfile:
	r.Register("gcp", gcp.New(nil))
	return r
}

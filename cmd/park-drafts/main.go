// Command park-drafts starts the local draft API with SQLite or memory storage.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shanecandoit/ProcessStarlark/internal/draftstore"
	"github.com/shanecandoit/ProcessStarlark/internal/project"
	"github.com/shanecandoit/ProcessStarlark/internal/server"
	"github.com/shanecandoit/ProcessStarlark/internal/workspace"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "park-drafts:", err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", "127.0.0.1:8081", "loopback listen address")
	root := flag.String("root", "examples/shipping", "read-only reference project directory")
	webDir := flag.String("web-dir", "", "optional built playground directory (run sh web/build.sh first)")
	dbPath := flag.String("db", ".park/drafts.sqlite", "SQLite database file; empty string uses temporary memory storage")
	backup := flag.String("backup", "", "write a consistent SQLite backup to a new file and exit")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments; use --help")
	}
	if err := validateAddress(*addr); err != nil {
		return err
	}
	p, err := project.LoadWith(*root, nil)
	if err != nil {
		return err
	}
	files := workspace.Files{}
	for name, content := range p.Files {
		files[name] = string(content)
	}
	var handler http.Handler
	storage := "memory (temporary)"
	if *dbPath == "" {
		if *backup != "" {
			return fmt.Errorf("--backup requires --db")
		}
		handler, err = server.NewDraftHandler(files)
	} else {
		store, openErr := draftstore.Open(*dbPath, files)
		if openErr != nil {
			return openErr
		}
		defer store.Close()
		if *backup != "" {
			if err := store.Backup(context.Background(), *backup); err != nil {
				return err
			}
			fmt.Printf("Backup saved: %s\n", *backup)
			return nil
		}
		handler, err = server.NewStoredDraftHandler(store)
		storage = "SQLite: " + *dbPath
	}
	if err != nil {
		return err
	}
	if *webDir != "" {
		handler, err = server.WithPlayground(handler, *webDir)
		if err != nil {
			return err
		}
	}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	fmt.Printf("Local draft API: http://%s\nReference project: %s\nStorage: %s\nLocal review, release activation, and rollback APIs are available. Actor labels are not authenticated accounts.\n", listener.Addr(), *root, storage)
	if err := srv.Serve(listener); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func validateAddress(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("use a loopback IP such as 127.0.0.1; this development service has no authentication")
	}
	return nil
}

// gcode-server runs the real Go engine without a display or MyGo dependency.
package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"gcode/internal/applog"
	"gcode/internal/engine"
	"gcode/internal/remote"
	"gcode/internal/storage"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}
func run() (result error) {
	var store *storage.Store
	var stopLogging func()
	errorReported := false
	defer func() {
		if result != nil && !errorReported {
			log.Print(result)
		}
		if stopLogging != nil {
			stopLogging()
		}
		if store != nil {
			if err := store.Close(); err != nil {
				log.Printf("close storage: %v", err)
			}
		}
	}()
	directory := flag.String("data-dir", "", "Fixed configuration/profile directory (default ~/.gcode); runtime data root is selected in settings")
	listen := flag.String("listen", "127.0.0.1:27655", "Authenticated HTTP/SSE listen address")
	showToken := flag.Bool("show-token", false, "Print the local remote API token and exit")
	certificate := flag.String("tls-cert", "", "TLS certificate PEM (optional behind a secure tunnel or reverse proxy)")
	privateKey := flag.String("tls-key", "", "TLS private key PEM")
	flag.Parse()
	var err error
	store, err = storage.Open(storage.Options{Directory: *directory})
	if err != nil {
		return err
	}
	stopLogging, err = applog.StartRuntime(store, os.Stderr)
	if err != nil {
		return err
	}
	tokenPath := filepath.Join(store.ConfigDirectory(), "server.json")
	var config struct {
		Token string `json:"token"`
	}
	data, err := os.ReadFile(tokenPath)
	if err == nil {
		if err = json.Unmarshal(data, &config); err != nil {
			return err
		}
	} else if errors.Is(err, os.ErrNotExist) {
		config.Token = rand.Text() + rand.Text()
		if err = storage.WriteJSON(tokenPath, config); err != nil {
			return err
		}
	} else {
		return err
	}
	if *showToken {
		fmt.Println(config.Token)
		return nil
	}
	core, err := engine.Open(engine.Options{Storage: store})
	if err != nil {
		return err
	}
	defer core.Close()
	// Engine.Close also closes the store. Persist the final error while its
	// registered root can still be resolved, then allow engine shutdown to run.
	defer func() {
		if result != nil {
			log.Print(result)
			errorReported = true
		}
	}()
	handler, err := (remote.Server{Core: core, Token: config.Token}).Handler()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 64 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	log.Printf("GCode server listening on %s; data: %s", listener.Addr(), store.Directory())
	if (*certificate == "") != (*privateKey == "") {
		return errors.New("provide both --tls-cert and --tls-key")
	}
	if *certificate != "" {
		err = server.ServeTLS(listener, *certificate, *privateKey)
	} else {
		err = server.Serve(listener)
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

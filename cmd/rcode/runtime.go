package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"gcode/internal/storage"
)

// A remote desktop has no local engine to own runtime maintenance. These paths
// belong to the client's registered store, independently of the remote host.
func startClientRuntimeMaintenance(store *storage.Store) (func(), error) {
	if err := storage.PrepareRuntime(store); err != nil {
		return nil, fmt.Errorf("prepare local client runtime: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	failures := storage.StartRuntimeMaintenance(ctx, store)
	done := make(chan struct{})
	go consumeClientRuntimeErrors(failures, done)
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}, nil
}

func consumeClientRuntimeErrors(failures <-chan error, done chan<- struct{}) {
	defer close(done)
	for err := range failures {
		log.Printf("local client runtime maintenance: %v", err)
	}
}

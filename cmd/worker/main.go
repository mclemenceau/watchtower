package main

import (
	"log"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/mclemenceau/argus/internal/config"
)

const taskQueue = "argus"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	c, err := client.Dial(client.Options{
		HostPort: cfg.TemporalHost,
	})
	if err != nil {
		log.Fatalf("worker: dial temporal: %v", err)
	}
	defer c.Close()

	w := worker.New(c, taskQueue, worker.Options{})

	// Workflows registered in later blocks — stubs keep the worker visible in the UI.
	w.RegisterWorkflow(StatusTableWorkflow)
	w.RegisterWorkflow(ChangeWatchWorkflow)
	w.RegisterWorkflow(QueryWorkflow)

	log.Printf("worker started on task queue %q (temporal: %s)", taskQueue, cfg.TemporalHost)
	if err := w.Run(worker.InterruptCh()); err != nil {
		log.Fatalf("worker: %v", err)
	}
}

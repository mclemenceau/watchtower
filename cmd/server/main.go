package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.temporal.io/sdk/client"

	"github.com/mclemenceau/argus/internal/buildapi"
	"github.com/mclemenceau/argus/internal/config"
	argusworkflow "github.com/mclemenceau/argus/internal/workflow"
)

const taskQueue = "argus"

// broadcaster fans out string messages to all subscribed SSE clients.
type broadcaster struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
}

func newBroadcaster() *broadcaster {
	return &broadcaster{clients: make(map[chan string]struct{})}
}

func (b *broadcaster) subscribe() chan string {
	ch := make(chan string, 16)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *broadcaster) unsubscribe(ch chan string) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
}

func (b *broadcaster) publish(msg string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- msg:
		default: // drop if client is slow
		}
	}
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	c, err := client.Dial(client.Options{
		HostPort: cfg.TemporalHost,
	})
	if err != nil {
		log.Fatalf("server: dial temporal: %v", err)
	}
	defer c.Close()

	b := newBroadcaster()

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	r.POST("/query", queryHandler(c))
	r.GET("/feed", feedHandler(b))
	r.POST("/internal/push", pushHandler(b))

	// Serve the web UI from the web/ directory.
	r.StaticFile("/", "web/index.html")

	log.Printf("server listening on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func queryHandler(c client.Client) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var req struct {
			Query string `json:"query" binding:"required"`
		}
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		timeoutCtx, cancel := context.WithTimeout(ctx.Request.Context(), 2*time.Minute)
		defer cancel()

		run, err := c.ExecuteWorkflow(timeoutCtx, client.StartWorkflowOptions{
			TaskQueue: taskQueue,
		}, argusworkflow.QueryWorkflow, req.Query)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var reply buildapi.AgentReply
		if err := run.Get(timeoutCtx, &reply); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		ctx.JSON(http.StatusOK, reply)
	}
}

func feedHandler(b *broadcaster) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ch := b.subscribe()
		defer b.unsubscribe(ch)

		ctx.Header("Content-Type", "text/event-stream")
		ctx.Header("Cache-Control", "no-cache")
		ctx.Header("Access-Control-Allow-Origin", "*")

		ctx.Stream(func(_ io.Writer) bool {
			select {
			case msg := <-ch:
				ctx.SSEvent("message", msg)
				return true
			case <-ctx.Request.Context().Done():
				return false
			}
		})
	}
}

func pushHandler(b *broadcaster) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var body struct {
			Data string `json:"data"`
		}
		if err := ctx.ShouldBindJSON(&body); err != nil {
			ctx.Status(http.StatusBadRequest)
			return
		}
		b.publish(body.Data)
		ctx.Status(http.StatusNoContent)
	}
}

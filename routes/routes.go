// Package routes defines the HTTP handlers for the ln-stream control panel.
// Handlers are protected by a mutex to prevent concurrent graph operations.
package routes

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/lightninglabs/lndclient"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
	"ln-stream/lnd"
	"ln-stream/memgraph"
)

var (
	// LndServices is the gRPC client for LND. Nil when running in snapshot-only mode.
	LndServices *lndclient.GrpcLndServices
	// Driver is the Neo4j/Memgraph database connection.
	Driver neo4j.Driver

	// mu protects isRoutineRunning and stopChannel from concurrent access.
	mu               sync.Mutex
	isRoutineRunning bool
	stopChannel      chan struct{}
)

// stopRoutine signals the graph update goroutine to stop. Must be called with mu held.
func stopRoutine() {
	if isRoutineRunning {
		close(stopChannel)
		isRoutineRunning = false
	}
}

// requireLND checks that LND is configured and returns a 400 error if not.
// Used to guard handlers that need a live LND connection.
func requireLND(c *gin.Context) bool {
	if LndServices == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "LND not configured"})
		return false
	}
	return true
}

// ToggleUpdatesHandler starts or stops the real-time graph update subscription.
// Requires LND to be configured.
func ToggleUpdatesHandler(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()

	if !requireLND(c) {
		return
	}

	if !isRoutineRunning {
		stopChannel = make(chan struct{})
		isRoutineRunning = true
		go subscribeToGraphUpdates(stopChannel)
		c.JSON(http.StatusOK, gin.H{"isRoutineRunning": true,
			"message": "Routine started."})
	} else {
		stopRoutine()
		c.JSON(http.StatusOK, gin.H{"isRoutineRunning": false,
			"message": "Routine stopped."})
	}
}

// ResetGraphHandler drops the database, pulls a fresh graph from LND, writes it
// to Memgraph, and runs post-import computations. Requires LND to be configured.
func ResetGraphHandler(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()

	if !requireLND(c) {
		return
	}

	log.Println("Graph update initiated...")
	stopRoutine()

	if err := memgraph.DropDatabase(Driver); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to drop database: %v", err)})
		return
	}
	graph, err := lnd.PullGraph(LndServices)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to pull graph: %v", err)})
		return
	}
	if err := lnd.WriteGraphToMemgraph(graph, Driver); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to write graph: %v", err)})
		return
	}
	if err := memgraph.SetupAfterImport(Driver); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("post-import setup failed: %v", err)})
		return
	}

	c.String(http.StatusOK, "Graph update complete.")
}

// LoadLocalSnapshot drops the database and loads the graph from a local
// describegraph.json snapshot. Does not require LND.
func LoadLocalSnapshot(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()

	log.Println("Snapshot load initiated...")
	stopRoutine()

	if err := memgraph.DropDatabase(Driver); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to drop database: %v", err)})
		return
	}
	if err := lnd.WriteSnapshotToMemgraph("./describegraph.json", Driver); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to load snapshot: %v", err)})
		return
	}
	if err := memgraph.SetupAfterImport(Driver); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("post-import setup failed: %v", err)})
		return
	}

	c.String(http.StatusOK, "Snapshot load complete.")
}

// GetStatusHandler returns whether the graph update routine is currently running.
func GetStatusHandler(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()

	c.JSON(http.StatusOK, gin.H{"isRoutineRunning": isRoutineRunning})
}

// subscribeToGraphUpdates subscribes to LND's graph topology update stream and
// applies each update to Memgraph. Runs until the stop channel is closed.
func subscribeToGraphUpdates(stop <-chan struct{}) {
	graphUpdates, errors, err := LndServices.Client.SubscribeGraph(context.Background())
	if err != nil {
		log.Printf("Failed to subscribe to graph updates: %v", err)
		mu.Lock()
		isRoutineRunning = false
		mu.Unlock()
		return
	}

	log.Println("Subscribed to graph topology updates. Waiting for updates...")
	for {
		select {
		case update := <-graphUpdates:
			memgraph.ProcessUpdates(Driver, update)
		case err := <-errors:
			log.Printf("Error receiving graph update: %v", err)
		case <-stop:
			log.Println("Stopping graph update loop.")
			return
		}
	}
}

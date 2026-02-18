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
	LndServices *lndclient.GrpcLndServices
	Driver      neo4j.Driver

	mu               sync.Mutex
	isRoutineRunning bool
	stopChannel      chan struct{}
)

func stopRoutine() {
	if isRoutineRunning {
		close(stopChannel)
		isRoutineRunning = false
	}
}

func ToggleUpdatesHandler(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()

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

func ResetGraphHandler(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()

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

func GetStatusHandler(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()

	c.JSON(http.StatusOK, gin.H{"isRoutineRunning": isRoutineRunning})
}

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

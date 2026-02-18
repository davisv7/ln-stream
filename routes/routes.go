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

	fmt.Println("Graph update initiated...")
	stopRoutine()

	memgraph.DropDatabase(Driver)
	graph := lnd.PullGraph(LndServices)
	lnd.WriteGraphToMemgraph(graph, Driver)
	memgraph.SetupAfterImport(Driver)

	c.String(http.StatusOK, "Graph update started.")
}

func LoadLocalSnapshot(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()

	fmt.Println("Graph update initiated...")
	stopRoutine()

	memgraph.DropDatabase(Driver)
	lnd.WriteSnapshotToMemgraph("./describegraph.json", Driver)
	memgraph.SetupAfterImport(Driver)

	c.String(http.StatusOK, "Graph update started.")
}

func GetStatusHandler(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()

	c.JSON(http.StatusOK, gin.H{"isRoutineRunning": isRoutineRunning,
		"message": "Routine stopped."})
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

	fmt.Println("Routine started.")
	fmt.Println("Subscribed to graph topology updates. Waiting for updates...")
	for {
		select {
		case update := <-graphUpdates:
			memgraph.ProcessUpdates(Driver, update)
		case err := <-errors:
			log.Printf("Error receiving graph update: %v\n", err)
		case <-stop:
			fmt.Println("Stopping graph update loop.")
			return
		}
	}
}

package routes

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/lightninglabs/lndclient"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
	"ln-stream/lnd"
	"ln-stream/memgraph"
	"log"
	"net/http"
)

var (
	IsRoutineRunning = true
	LndServices      *lndclient.GrpcLndServices
	Driver           neo4j.Driver
	StopChannel      chan bool
	GraphUpdates     <-chan *lndclient.GraphTopologyUpdate
	Errors           <-chan error
)

func ToggleUpdatesHandler(c *gin.Context) {
	var err error
	// Subscribe to graph topology updates
	GraphUpdates, Errors, err = LndServices.Client.SubscribeGraph(context.Background())
	if err != nil {
		log.Fatalf("Failed to subscribe to graph updates: %v", err)
	}

	if !IsRoutineRunning {
		IsRoutineRunning = true
		StopChannel = make(chan bool)
		go SubscribeToGraphUpdates()
		c.JSON(http.StatusOK, gin.H{"isRoutineRunning": true,
			"message": "Routine started."})
	} else {
		IsRoutineRunning = false
		StopChannel <- true
		close(StopChannel)
		c.JSON(http.StatusOK, gin.H{"isRoutineRunning": false,
			"message": "Routine stopped."})
	}
}

func ResetGraphHandler(c *gin.Context) {
	fmt.Println("Graph update initiated...")

	if IsRoutineRunning {
		IsRoutineRunning = false
		StopChannel <- true
		close(StopChannel)
	}

	memgraph.DropDatabase(Driver)
	graph := lnd.PullGraph(LndServices)
	lnd.WriteGraphToMemgraph(graph, Driver)
	memgraph.SetupAfterImport(Driver)

	StopChannel = make(chan bool)

	c.String(http.StatusOK, "Graph update started.")
}

func LoadLocalSnapshot(c *gin.Context) {
	fmt.Println("Graph update initiated...")

	if IsRoutineRunning {
		IsRoutineRunning = false
		StopChannel <- true
		close(StopChannel)
	}

	memgraph.DropDatabase(Driver)
	lnd.WriteSnapshotToMemgraph("./describegraph.json", Driver)
	memgraph.SetupAfterImport(Driver)

	c.String(http.StatusOK, "Graph update started.")
}

func GetStatusHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"isRoutineRunning": IsRoutineRunning,
		"message": "Routine stopped."})
}

func SubscribeToGraphUpdates() {
	var err error
	GraphUpdates, Errors, err = LndServices.Client.SubscribeGraph(context.Background())
	if err != nil {
		log.Fatalf("Failed to subscribe to graph updates: %v", err)
	}
	IsRoutineRunning = true
	StopChannel = make(chan bool)
	fmt.Println("Routine started.")
	fmt.Println("Subscribed to graph topology updates. Waiting for updates...")
	for {
		select {
		case update := <-GraphUpdates:
			memgraph.ProcessUpdates(Driver, update)
		case err := <-Errors:
			log.Printf("Error receiving graph update: %v\n", err)
		case <-StopChannel:
			fmt.Println("Stopping graph update loop.")
			return
		}
	}
}

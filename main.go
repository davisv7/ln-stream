// Package main initializes the ln-stream application, which syncs Lightning Network
// graph topology from LND into a Memgraph database and serves a control panel UI.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"ln-stream/lnd"
	"ln-stream/memgraph"
	"ln-stream/routes"
)

func main() {
	var err error

	// Load .env if present; ignored in Docker where env vars are set via compose.
	_ = godotenv.Load(".env")

	// Connect to Memgraph (required).
	routes.Driver, err = memgraph.ConnectNeo4j()
	if err != nil {
		log.Fatalf("Failed to connect to Neo4j: %v", err)
	}
	defer memgraph.CloseDriver(routes.Driver)

	// Connect to LND if configured. Without LND, only snapshot loading is available.
	if os.Getenv("LND_ADDRESS") != "" {
		routes.LndServices, err = lnd.ConnectToLND()
		if err != nil {
			log.Printf("Failed to connect to LND: %v (snapshot-only mode)", err)
		} else {
			defer routes.LndServices.Close()
		}
	} else {
		log.Println("LND_ADDRESS not set, running in snapshot-only mode")
	}

	// Set up HTTP routes and static file serving.
	router := gin.Default()
	router.GET("/reset-graph", routes.ResetGraphHandler)
	router.GET("/load-local-snapshot", routes.LoadLocalSnapshot)
	router.GET("/toggle-updates", routes.ToggleUpdatesHandler)
	router.GET("/get-status", routes.GetStatusHandler)
	router.StaticFile("/static/script.js", "./static/script.js")
	router.StaticFile("/static/style.css", "./static/style.css")
	router.StaticFile("/", "./index.html")

	fmt.Println("Server started at http://localhost:8080")
	router.Run(":8080")
}

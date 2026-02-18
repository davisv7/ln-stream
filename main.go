package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"ln-stream/lnd"
	"ln-stream/memgraph"
	"ln-stream/routes"
	"log"
)

func main() {
	var err error
	err = godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Failed to load .env file: %v", err)
	}

	// Connect to Neo4j instance and drop the database
	routes.Driver, err = memgraph.ConnectNeo4j()
	if err != nil {
		log.Fatalf("Failed to connect to Neo4j: %v", err)
	}
	defer memgraph.CloseDriver(routes.Driver)

	// Connect to lnd
	routes.LndServices, err = lnd.ConnectToLND()
	if err != nil {
		log.Fatalf("Failed to create new lnd services: %v", err)
	}
	defer routes.LndServices.Close()

	router := gin.Default()
	// Handle the button click
	router.GET("/reset-graph", routes.ResetGraphHandler)
	router.GET("/load-local-snapshot", routes.LoadLocalSnapshot)

	// Handle the button click to toggle the routine
	router.GET("/toggle-updates", routes.ToggleUpdatesHandler)

	router.GET("/get-status", routes.GetStatusHandler)

	// Serve static files (HTML, CSS, JS)
	router.StaticFile("/static/script.js", "./static/script.js")
	router.StaticFile("/static/style.css", "./static/style.css")
	router.StaticFile("/", "./index.html")

	routes.IsRoutineRunning = false

	// Start the server
	fmt.Println("Server started at http://localhost:8080")
	router.Run(":8080")

}

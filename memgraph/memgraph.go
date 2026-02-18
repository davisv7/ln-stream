package memgraph

import (
	"fmt"
	"log"
	"os"

	"github.com/lightninglabs/lndclient"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
)

// ConnectNeo4j creates a new Neo4j driver instance and establishes a connection
func ConnectNeo4j() (neo4j.Driver, error) {
	host := os.Getenv("NEO4J_HOST")
	port := os.Getenv("NEO4J_PORT")
	scheme := "bolt://"
	if host != "localhost" && host != "127.0.0.1" && host != "memgraph-mage" {
		scheme = "bolt+ssc://"
	}

	uri := scheme + host + ":" + port
	username := os.Getenv("NEO4J_USERNAME")
	password := os.Getenv("NEO4J_PASSWORD")

	driver, err := neo4j.NewDriver(uri, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		return nil, fmt.Errorf("failed to create Neo4j driver: %v", err)
	}

	return driver, nil
}

// CloseDriver closes the Neo4j driver connection
func CloseDriver(driver neo4j.Driver) {
	driver.Close()
}

// DropDatabase drops all nodes/relationships from the database.
func DropDatabase(neo4jDriver neo4j.Driver) error {
	log.Println("Dropping database...")
	session := neo4jDriver.NewSession(neo4j.SessionConfig{})
	defer session.Close()

	_, err := session.Run("MATCH (n) DETACH DELETE n", nil)
	if err != nil {
		return fmt.Errorf("failed to drop database: %w", err)
	}

	// Drop index on pub_key property
	_, err = session.Run("DROP INDEX ON :node(pubkey)", nil)
	if err != nil {
		log.Printf("Failed to drop index on pubkey property: %v", err)
	}

	// Drop index on channel_id property
	_, err = session.Run("DROP INDEX ON :edge(channel_id)", nil)
	if err != nil {
		log.Printf("Failed to drop index on channel_id property: %v", err)
	}

	return nil
}

// CommitQuery commits a query to Neo4j with parameters
func CommitQuery(driver neo4j.Driver, query string, params map[string]interface{}) (neo4j.Result, error) {
	session := driver.NewSession(neo4j.SessionConfig{})
	defer session.Close()
	result, err := session.Run(query, params)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	return result, nil
}

// ProcessNodeUpdate converts node updates to Memgraph queries
func ProcessNodeUpdate(nodeUpdate lndclient.NodeUpdate) (string, map[string]interface{}) {
	nodeQuery := "MERGE (n:node {pubkey: $pubKey})\nSET n.alias = $alias"
	params := map[string]interface{}{
		"pubKey": nodeUpdate.IdentityKey.String(),
		"alias":  nodeUpdate.Alias,
	}
	return nodeQuery, params
}

// ProcessEdgeUpdate converts channel edge updates to Memgraph queries
func ProcessEdgeUpdate(edgeUpdate lndclient.ChannelEdgeUpdate) (string, map[string]interface{}) {
	var (
		edgeQuery string
		params    map[string]interface{}
	)
	if edgeUpdate.RoutingPolicy.Disabled {
		edgeQuery = "MATCH ()-[r:edge {channel_id: $channelID}]->()\nset r.disabled = true"
		params = map[string]interface{}{
			"channelID": edgeUpdate.ChannelID.String(),
		}
	} else {
		edgeQuery = "MERGE (n1:node {pubkey: $advertisingNode})\nMERGE (n2:node {pubkey: $connectingNode})\n" +
			"MERGE (n1)-[r:edge {channel_id: $channelID}]->(n2)\n" +
			"SET r.fee_base_msat = $fee_base_msat, r.fee_rate_milli_msat = $fee_rate_milli_msat, r.time_lock_delta = $time_lock_delta, r.disabled = $disabled"
		params = map[string]interface{}{
			"advertisingNode":     edgeUpdate.AdvertisingNode.String(),
			"connectingNode":      edgeUpdate.ConnectingNode.String(),
			"channelID":           edgeUpdate.ChannelID.String(),
			"capacity":            edgeUpdate.Capacity,
			"fee_base_msat":       edgeUpdate.RoutingPolicy.FeeBaseMsat,
			"fee_rate_milli_msat": edgeUpdate.RoutingPolicy.FeeRateMilliMsat,
			"time_lock_delta":     edgeUpdate.RoutingPolicy.TimeLockDelta,
			"disabled":            edgeUpdate.RoutingPolicy.Disabled,
		}
	}
	return edgeQuery, params
}

// ProcessCloseUpdate converts channel close updates to Memgraph queries
func ProcessCloseUpdate(closeUpdate lndclient.ChannelCloseUpdate) (string, map[string]interface{}) {
	closeQuery := "MATCH ()-[r:edge {channel_id: $channelID}]->()\nDELETE r"
	params := map[string]interface{}{
		"channelID": closeUpdate.ChannelID.String(),
	}
	return closeQuery, params
}

// ProcessUpdates calls the corresponding update function for each type of update
func ProcessUpdates(driver neo4j.Driver, update *lndclient.GraphTopologyUpdate) {
	for _, nodeUpdate := range update.NodeUpdates {
		nodeQuery, nodeParams := ProcessNodeUpdate(nodeUpdate)
		_, err := CommitQuery(driver, nodeQuery, nodeParams)
		if err != nil {
			log.Printf("Failed to commit node query: %v", err)
		}
	}

	for _, edgeUpdate := range update.ChannelEdgeUpdates {
		edgeQuery, edgeParams := ProcessEdgeUpdate(edgeUpdate)
		_, err := CommitQuery(driver, edgeQuery, edgeParams)
		if err != nil {
			log.Printf("Failed to commit edge query: %v", err)
		}
	}

	for _, closeUpdate := range update.ChannelCloseUpdates {
		closeQuery, closeParams := ProcessCloseUpdate(closeUpdate)
		_, err := CommitQuery(driver, closeQuery, closeParams)
		if err != nil {
			log.Printf("Failed to commit close query: %v", err)
		}
	}
}

// SetupAfterImport runs a set of commands directly after importing a snapshot.
func SetupAfterImport(neo4jDriver neo4j.Driver) error {
	log.Println("Running post-import setup...")
	session := neo4jDriver.NewSession(neo4j.SessionConfig{})
	defer session.Close()

	queries := []struct {
		desc  string
		query string
	}{
		{"fix fee denominations", "match (n)-[r]->(m)\nset r.fee_base_milli_msat = r.fee_base_msat*1000"},
		{"initialize node capacity", "match (n)\nset n.total_capacity = 0;\n"},
		{"calculate node capacity", "MATCH (n)-[r]-(m)\nWITH n,sum(r.capacity) as total_capacity\nSET n.total_capacity = total_capacity/2;"},
		{"calculate node betweenness centrality", "call betweenness_centrality.get() YIELD betweenness_centrality, node \nwith betweenness_centrality,node\nset node.betweenness_centrality = betweenness_centrality;"},
		{"calculate edge betweenness centrality", "MATCH (n)-[r]-(m)\nset r.betweenness_centrality = (n.betweenness_centrality+m.betweenness_centrality)/2;"},
	}

	for _, q := range queries {
		_, err := session.Run(q.query, nil)
		if err != nil {
			return fmt.Errorf("failed to %s: %w", q.desc, err)
		}
	}

	log.Println("Post-import setup complete.")
	return nil
}

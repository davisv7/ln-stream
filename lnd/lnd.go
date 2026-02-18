package lnd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/lightninglabs/lndclient"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
)

func convertChannelIDToString(channelID uint64) string {
	blockHeight := channelID >> 40
	blockIndex := (channelID >> 16) & ((1 << 24) - 1)
	outputIndex := channelID & ((1 << 16) - 1)
	return fmt.Sprintf("%d:%d:%d", blockHeight, blockIndex, outputIndex)
}

func ConnectToLND() (*lndclient.GrpcLndServices, error) {
	config := lndclient.LndServicesConfig{
		LndAddress:         os.Getenv("LND_ADDRESS"),
		Network:            lndclient.Network(os.Getenv("LND_NETWORK")),
		CustomMacaroonPath: os.Getenv("LND_MACAROON_PATH"),
		TLSPath:            os.Getenv("LND_TLS_CERT_PATH"),
	}
	return lndclient.NewLndServices(&config)
}

type Node struct {
	Pub_Key string

	LastUpdate time.Time

	Alias string

	Color string

	Features map[string]interface{}

	Addresses []interface{}
}

type ChannelEdge struct {
	ChannelId uint64 `json:"channel_id,string"`

	Capacity string `json:"capacity"`

	Node1_Pub string `json:"node1_pub"`

	Node2_Pub string `json:"node2_pub"`

	Node1Policy RoutingPolicy `json:"node1_policy,omitempty"`

	Node2Policy RoutingPolicy `json:"node2_policy,omitempty"`
}

// RoutingPolicy holds the edge routing policy for a channel edge.
type RoutingPolicy struct {
	TimeLockDelta    int    `json:"time_lock_delta"`
	MinHtlc          string `json:"min_htlc"`
	FeeBaseMsat      string `json:"fee_base_msat"`
	FeeRateMilliMsat string `json:"fee_rate_milli_msat"`
	Disabled         bool   `json:"disabled"`
	MaxHtlcMsat      string `json:"max_htlc_msat"`
	LastUpdate       int    `json:"last_update"`
	CustomRecords    struct {
	} `json:"custom_records"`
}

// Graph describes our view of the graph.
type Graph struct {
	// Nodes is the set of nodes in the channel graph.
	Nodes []Node

	// Edges is the set of edges in the channel graph.
	Edges []ChannelEdge
}

func writeNodesToMemgraph(session neo4j.Session, nodes []lndclient.Node) {
	const batchSize = 100

	for i := 0; i < len(nodes); i += batchSize {
		end := i + batchSize
		if end > len(nodes) {
			end = len(nodes)
		}
		batch := nodes[i:end]

		// Prepare the list of maps for UNWIND
		records := make([]map[string]interface{}, 0, len(batch))
		for _, node := range batch {
			records = append(records, map[string]interface{}{
				"pubKey":    node.PubKey.String(),
				"alias":     node.Alias,
				"addresses": node.Addresses,
			})
		}

		query := `
			UNWIND $rows AS row
			MERGE (n:node {pubkey: row.pubKey})
			SET n.alias = row.alias, n.addresses = row.addresses
		`

		params := map[string]interface{}{"rows": records}

		_, err := session.Run(query, params)
		if err != nil {
			log.Printf("Failed to execute batch node query: %v", err)
		}
	}
}

func createNodeIndex(session neo4j.Session) {
	// Query to create an index on the pubkey property of Node
	indexQuery := "CREATE INDEX ON :node(pubkey)"

	// Execute the index creation query
	_, err := session.Run(indexQuery, nil)
	if err != nil {
		log.Printf("Failed to create index: %v", err)
	}
}

func createIndexForChannels(session neo4j.Session) {
	// Query to create an index on the channel_id property of CHANNEL relationships
	indexQuery := "CREATE INDEX ON :edge(channel_id)"

	// Execute the index creation query
	_, err := session.Run(indexQuery, nil)
	if err != nil {
		log.Printf("Failed to create index for channels: %v", err)
	}
}

func writeChannelsToMemgraph(session neo4j.Session, edges []lndclient.ChannelEdge) {
	const batchSize = 100

	relations := []map[string]interface{}{}

	for _, edge := range edges {
		chanID := strings.Replace(convertChannelIDToString(edge.ChannelID), ":", "x", -1)

		if edge.Node1Policy != nil {
			relations = append(relations, map[string]interface{}{
				"from":          edge.Node1.String(),
				"to":            edge.Node2.String(),
				"chan_id":       chanID,
				"capacity":      edge.Capacity,
				"fee_base":      edge.Node1Policy.FeeBaseMsat,
				"fee_rate":      edge.Node1Policy.FeeRateMilliMsat,
				"time_lock":     edge.Node1Policy.TimeLockDelta,
				"disabled":      edge.Node1Policy.Disabled,
				"min_htlc":      edge.Node1Policy.MinHtlcMsat,
				"max_htlc":      edge.Node1Policy.MaxHtlcMsat,
				"min_liquidity": 0,
				"max_liquidity": edge.Capacity,
			})
		}

		if edge.Node2Policy != nil {
			relations = append(relations, map[string]interface{}{
				"from":          edge.Node2.String(),
				"to":            edge.Node1.String(),
				"chan_id":       chanID,
				"capacity":      edge.Capacity,
				"fee_base":      edge.Node2Policy.FeeBaseMsat,
				"fee_rate":      edge.Node2Policy.FeeRateMilliMsat,
				"time_lock":     edge.Node2Policy.TimeLockDelta,
				"disabled":      edge.Node2Policy.Disabled,
				"min_htlc":      edge.Node2Policy.MinHtlcMsat,
				"max_htlc":      edge.Node2Policy.MaxHtlcMsat,
				"min_liquidity": 0,
				"max_liquidity": edge.Capacity,
			})
		}
	}

	for i := 0; i < len(relations); i += batchSize {
		end := i + batchSize
		if end > len(relations) {
			end = len(relations)
		}

		batch := relations[i:end]
		query := `
			UNWIND $rows AS row
			MATCH (a:node {pubkey: row.from}), (b:node {pubkey: row.to})
			MERGE (a)-[r:edge {channel_id: row.chan_id, capacity: row.capacity}]->(b)
			SET r.fee_base_msat = row.fee_base,
				r.fee_rate_milli_msat = row.fee_rate,
				r.time_lock_delta = row.time_lock,
				r.disabled = row.disabled,
				r.min_htlc_msat = row.min_htlc,
				r.max_htlc_msat = row.max_htlc,
			    r.min_liquidity = row.min_liquidity,
			    r.max_liquidity = row.max_liquidity
		`

		params := map[string]interface{}{"rows": batch}
		_, err := session.Run(query, params)
		if err != nil {
			log.Printf("Failed to execute batch channel policy query: %v", err)
		}
	}
}

func PullGraph(lndServices *lndclient.GrpcLndServices) *lndclient.Graph {

	fmt.Println("Pulling graph...")
	duration := 10 * 60 * time.Second
	_ctx := context.WithoutCancel(context.Background())
	ctx, cancel := context.WithTimeout(_ctx, duration)
	defer cancel()
	graph, err := lndServices.Client.DescribeGraph(ctx, false)
	if err != nil {
		log.Printf("Failed to execute channel policy query: %v", err)
	}
	return graph

}

func WriteGraphToMemgraph(graph *lndclient.Graph, neo4jDriver neo4j.Driver) {
	session := neo4jDriver.NewSession(neo4j.SessionConfig{})
	defer session.Close()

	fmt.Println("Writing to Memgraph...")
	createNodeIndex(session)
	createIndexForChannels(session)
	writeNodesToMemgraph(session, graph.Nodes)
	writeChannelsToMemgraph(session, graph.Edges)
	fmt.Println("Finished writing to Memgraph...")
}

func WriteSnapshotToMemgraph(snapshotFilename string, neo4jDriver neo4j.Driver) {
	session := neo4jDriver.NewSession(neo4j.SessionConfig{})
	defer session.Close()

	jsonFile, err := os.Open(snapshotFilename)
	if err != nil {
		log.Fatalf("Failed to open snapshot: %v", err)
	}
	defer jsonFile.Close()

	byteValue, _ := io.ReadAll(jsonFile)
	var graph Graph
	err = json.Unmarshal(byteValue, &graph)
	if err != nil {
		log.Fatalf("Failed to open snapshot: %v", err)
	}

	fmt.Println("Writing to Memgraph...")
	createNodeIndex(session)
	createIndexForChannels(session)
	writeSnapshotNodesToMemgraph(session, graph.Nodes)
	writeSnapshotChannelsToMemgraph(session, graph.Edges)
	fmt.Println("Finished writing to Memgraph...")
}

func writeSnapshotNodesToMemgraph(session neo4j.Session, nodes []Node) {
	for _, node := range nodes {
		_, is_wumbo := node.Features["19"]

		query := "MERGE (n:node {pubkey: $pubKey, alias: $alias, is_wumbo: $is_wumbo})"
		params := map[string]interface{}{
			"pubKey":   node.Pub_Key,
			"alias":    node.Alias,
			"is_wumbo": is_wumbo,
		}
		_, err := session.Run(query, params)
		if err != nil {
			log.Printf("Failed to execute node query: %v", err)
		}
	}
}

func writeSnapshotChannelsToMemgraph(session neo4j.Session, edges []ChannelEdge) {
	for _, edge := range edges {
		chanID := convertChannelIDToString(edge.ChannelId) // Convert uint64 to string format
		writeChannelPolicyToMemgraphSnapshot(session, &edge, edge.Node1Policy, edge.Node1_Pub, edge.Node2_Pub, chanID)
		writeChannelPolicyToMemgraphSnapshot(session, &edge, edge.Node2Policy, edge.Node2_Pub, edge.Node1_Pub, chanID)
	}
}

func writeChannelPolicyToMemgraphSnapshot(session neo4j.Session, edge *ChannelEdge, policy RoutingPolicy, node1PubKey, node2PubKey, chanID string) {
	if policy.MaxHtlcMsat != "" {
		query := `
          MATCH (a:node {pubkey: $node1}), (b:node {pubkey: $node2})
          MERGE (a)-[r:edge {channel_id: $chanID, capacity: $capacity}]->(b)
          SET r.fee_base_msat = $feeBase, r.fee_rate_milli_msat = $feeRate, r.time_lock_delta = $timeLock,
			r.disabled = $disabled, r.min_htlc_msat = $minHtlc, r.max_htlc_msat = $maxHtlc
		`
		params := map[string]interface{}{
			"node1":    node1PubKey,
			"node2":    node2PubKey,
			"chanID":   chanID,
			"capacity": edge.Capacity,
			"feeBase":  policy.FeeBaseMsat,
			"feeRate":  policy.FeeRateMilliMsat,
			"timeLock": policy.TimeLockDelta,
			"disabled": policy.Disabled,
			"minHtlc":  policy.MinHtlc,
			"maxHtlc":  policy.MaxHtlcMsat,
		}
		_, err := session.Run(query, params)
		if err != nil {
			log.Printf("Failed to execute channel policy query: %v", err)
		}
	}
}

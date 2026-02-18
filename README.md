# ln-stream

> Stream graph updates from LND into Memgraph.

ln-stream pulls your Lightning Network node's channel graph into a Memgraph database and keeps it updated in real time. This lets you run graph queries using Memgraph Lab's built-in Cypher console. A bundled snapshot is included so you can explore the graph without running a node.

## Quick Start (no LND)

```
git clone https://github.com/davisv7/ln-stream
cd ln-stream
docker compose up
```

1. Open the control panel at `localhost:8080`
2. Click **Load Local Snapshot** to import the bundled `describegraph.json`
3. Open Memgraph Lab at `localhost:3000` to explore the graph

## Quick Start (with LND)

```
git clone https://github.com/davisv7/ln-stream
cd ln-stream
```

1. Copy your `readonly.macaroon` and `tls.cert` into the `creds/` directory
2. Create your `.env` from the example and set your node's address and network:
   ```
   cp .env-example .env
   ```
   ```
   LND_ADDRESS=<HOST>:10009
   LND_NETWORK=mainnet
   ```
3. Start everything:
   ```
   docker compose up
   ```
4. Open the control panel at `localhost:8080`

## Control Panel

The control panel at `localhost:8080` has three actions:

- **Reset Graph** — clears the database and pulls a fresh graph from LND (requires LND)
- **Toggle Updates** — starts or stops the real-time graph subscription (requires LND)
- **Load Local Snapshot** — loads the bundled `describegraph.json` into Memgraph (no LND needed)

## Memgraph Lab

Memgraph Lab is available at `localhost:3000`.

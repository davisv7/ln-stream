# ln-stream

> Stream graph updates from LND into Memgraph.

## Setup and Installation

This guide assumes that you already have [Go](https://go.dev/) and [Memgraph Lab](https://memgraph.com/download#individual) installed.
To get started, follow these steps:

1. Clone this repository to your local machine:

   ```
   git clone https://git.lyberry.com/v/ln-stream.git
   cd ln-stream
   ```

2. Initialize the Go module and download dependencies:

   ```
   go mod tidy
   ```

3. Configure the environment variables:

    - Copy the example file `.env-example` to `.env`:

      ```
      cp .env-example .env
      ```

    - Open the `.env` file and populate it with the relevant values:

      ```
      NEO4J_HOST=localhost  
      NEO4J_PORT=7687  
      NEO4J_USERNAME=
      NEO4J_PASSWORD=
      LND_ADDRESS=<HOST>:10009
      LND_NETWORK=mainnet
      LND_MACAROON_PATH=/path/to/readonly.macaroon
      LND_TLS_CERT_PATH=/path/to/tls.cert
      ```

4. Start Memgraph using Docker Compose:

   ```
   docker compose up
   ```

5. Start the Control Panel:

    ```
   go run main.go
   ```

From here, you should have an instance of memgraph up receiving updates from LND.
There is also a control panel running at http://localhost:8080 that lets you pause graph updates and reset the graph.
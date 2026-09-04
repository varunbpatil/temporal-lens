# Temporal Lens

Deep search and filter [Temporal.io](https://temporal.io/) workflows powered by [OpenSearch](https://github.com/opensearch-project/opensearch).

## Project Structure

This project follows hexagonal architecture (ports-and-adapters pattern).

```
.
├── Makefile                   # Task runner
├── buf.yaml                   # Buf module config
├── buf.gen.yaml               # Buf code generation config
│
├── config/                    # App configuration
│   └── config.go
│
├── protos/                    # Protobuf definitions
│   ├── src/                   # Proto source files
│   └── gen/                   # Generated code
│
├── domains/                   # App domains
│   └── workflows/
│       ├── models/            # Domain models
│       │   └── workflows.go
│       ├── ports/             # Domain ports
│       │   └── workflows.go
│       ├── service/           # Domain service
│       │   └── workflows.go
│       └── errors.go          # Domain errors
│
├── inbound/                   # Inbound adapters
│   ├── grpc/
│   ├── http/
│   └── mcp/
│
├── outbound/                  # Outbound adapters
│   ├── opensearch/
│   └── temporal/
│
└── ui/                        # React frontend
```

## Development

This project uses [mise](https://mise.jdx.dev) to manage the development environment and a [Makefile](./Makefile) to automate common tasks.

[tilt](https://tilt.dev/) is the process runner.

```sh
# Install dependencies
mise install

# See available tasks
make help

# Source environment variables (make your modifications to .env)
cp .env.example .env
source .env

# Start development server
tilt up
```

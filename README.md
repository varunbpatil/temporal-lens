<div align="center">

# 🔭 Temporal Lens

**Deep search and filtering of [Temporal.io](https://temporal.io/) workflows powered by [OpenSearch](https://github.com/opensearch-project/opensearch)**

[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?style=flat&logo=go&logoColor=white)](https://go.dev)
[![React](https://img.shields.io/badge/React-19-61DAFB?style=flat&logo=react&logoColor=black)](https://react.dev)
[![OpenSearch](https://img.shields.io/badge/OpenSearch-3.x-46398A?style=flat&logo=opensearch&logoColor=white)](https://opensearch.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)

</div>

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
├── types/                     # Custom type definitions
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

This project uses:

* [mise](https://mise.jdx.dev) to manage the development environment.
* [Makefile](./Makefile) as the task runner.
* [tilt](https://tilt.dev/) as the process runner.

## Usage

### Docker

```sh
# Build the image
make docker/build

# Run with environment variables from .env file (copy .env.example and modify it)
docker run --env-file .env temporal-lens
```

### Environment Variables

| Variable                            | Description                                           | Required | Default |
| ----------------------------------- | ----------------------------------------------------- | -------- | ------- |
| `LOG_LEVEL`                         | Log level (`debug`, `info`, `warn`, `error`)          | No       | `info`  |
| `LOG_FORMAT`                        | Log format (`text`, `json`)                           | No       | `text`  |
| `OPENSEARCH_ADDRESSES`              | Comma-separated OpenSearch addresses                  | Yes      |         |
| `OPENSEARCH_USERNAME`               | OpenSearch username                                   | No       |         |
| `OPENSEARCH_PASSWORD`               | OpenSearch password                                   | No       |         |
| `OPENSEARCH_API_KEY`                | OpenSearch API key (alternative to username/password) | No       |         |
| `OPENSEARCH_INSECURE_SKIP_VERIFY`   | Skip TLS certificate verification                     | No       | `false` |

## License

[MIT](./LICENSE)

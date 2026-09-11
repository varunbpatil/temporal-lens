<div align="center">

# 🔭 Temporal Lens

**Search engine for [Temporal.io](https://temporal.io/) workflows powered by [OpenSearch](https://github.com/opensearch-project/opensearch)**

[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?style=flat&logo=go&logoColor=white)](https://go.dev)
[![React](https://img.shields.io/badge/React-19-61DAFB?style=flat&logo=react&logoColor=black)](https://react.dev)
[![OpenSearch](https://img.shields.io/badge/OpenSearch-3.x-46398A?style=flat&logo=opensearch&logoColor=white)](https://opensearch.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)

</div>

## Features

### 🔍 Intuitive structured search

Construct complex queries with logical operators easily from the UI and search across namespaces. Queries can be constructed using:

* Workflow metadata - ID, Type, Namespace, Start/End time, Status, Search Attributes and errors.
* Activity metadata - Type/Custom activity ID, Attempts, Start/End time and errors.
* Child workflow metadata - ID, Type, Namespace, Attempts, Start/End time and errors

### 📝 Custom queries on JSON payloads

Easily create custom deployments that allow queries on custom fields extracted from JSON payloads of workflows, child workflows and activities.

### 🗂️ Unified view

Provides a unified view across all namespaces, even for Temporal Cloud. Queries filter across all namespaces. No more switching between namespaces in the Temporal UI.

### 📦 Bulk operations

Select individual workflows, a page of results, or all matching results to signal, reset, cancel, or terminate workflows using Temporal batch operations. Unlike the Temporal UI, this works across namespaces.

## Upcoming Features

* 🔒 Authentication
* 🔌 MCP server
* ⌨️ CLI tool and AI agent skill
* 🗃️ Admin dashboard to manage OpenSearch indexes and much more...

## Project Structure

This project follows hexagonal architecture (Ports & Adapters pattern).

```
.
├── Makefile                   # Task runner
├── buf.yaml                   # Buf module config
├── buf.gen.yaml               # Buf code generation config
│
├── config/                    # App configuration
├── mocks/                     # Generated GoMock implementations
├── types/                     # Custom type definitions
│
├── protos/                    # Protobuf definitions
│   ├── src/                   # Proto source files
│   └── gen/                   # Generated code
│
├── domains/                   # App domains
│   └── workflows/
│       ├── models/            # Domain models
│       ├── ports/             # Domain ports
│       ├── service/           # Domain service
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

| Variable                               | Description                                                           | Required    | Default      |
| -------------------------------------- | --------------------------------------------------------------------- | ----------- | --------     |
| `READ_ONLY`                            | Disable workflow mutations, including bulk actions and index deletion | No          | `false`      |
| `LOG_LEVEL`                            | Log level (`debug`, `info`, `warn`, `error`)                          | No          | `info`       |
| `LOG_FORMAT`                           | Log format (`text`, `json`)                                           | No          | `text`       |
| `GRPC_ADDRESS`                         | gRPC server listen address                                            | No          | `:50051`     |
| `HTTP_ADDRESS`                         | HTTP server listen address                                            | No          | `:8080`      |
| `OPENSEARCH_ADDRESSES`                 | Comma-separated OpenSearch addresses                                  | Yes         |              |
| `OPENSEARCH_USERNAME`                  | OpenSearch username                                                   | No          |              |
| `OPENSEARCH_PASSWORD`                  | OpenSearch password                                                   | No          |              |
| `OPENSEARCH_API_KEY`                   | OpenSearch API key (alternative to username/password)                 | No          |              |
| `OPENSEARCH_INSECURE_SKIP_VERIFY`      | Skip TLS certificate verification                                     | No          | `false`      |
| `TEMPORAL_NAMESPACES`                  | Comma-separated Temporal namespaces                                   | Yes         |              |
| `TEMPORAL_CLOUD`                       | Use Temporal Cloud endpoints                                          | No          | `false`      |
| `TEMPORAL_ACCOUNT`                     | Temporal Cloud account name                                           | Conditional |              |
| `TEMPORAL_ENDPOINT`                    | Self-hosted Temporal gRPC endpoint                                    | Conditional |              |
| `TEMPORAL_BASE_URL`                    | Temporal UI base URL                                                  | Yes         |              |
| `TEMPORAL_API_KEY`                     | API key sent as a Bearer token                                        | No          |              |
| `TEMPORAL_TLS`                         | Enable TLS for a self-hosted endpoint                                 | No          | `false`      |
| `TEMPORAL_INSECURE_SKIP_VERIFY`        | Skip Temporal TLS certificate verification                            | No          | `false`      |
| `TEMPORAL_SERVER_NAME`                 | TLS server name override                                              | No          |              |
| `TEMPORAL_CA_FILE`                     | PEM file containing trusted Temporal certificate authorities          | No          |              |
| `TEMPORAL_CLIENT_CERT_FILE`            | Client certificate file for mTLS                                      | No          |              |
| `TEMPORAL_CLIENT_KEY_FILE`             | Client private-key file for mTLS                                      | No          |              |
| `TEMPORAL_CONN_POOL_SIZE`              | gRPC connections per namespace                                        | No          | `1`          |
| `TEMPORAL_BULK_ACTIONS_PER_SECOND`     | Maximum operations/sec for Signal, Terminate, and Reset per namespace | No          | `8`          |
| `TEMPORAL_LIST_REQUESTS_PER_SECOND`    | Maximum workflow list RPCs per second per namespace                   | No          | `8`          |
| `TEMPORAL_HISTORY_REQUESTS_PER_SECOND` | Maximum workflow history RPCs per second per namespace                | No          | `8`          |
| `TEMPORAL_INDEX_PREFIX`                | Prefix for versioned daily OpenSearch workflow shard indexes          | No          | `workflows-` |
| `TEMPORAL_INDEX_VERSION`               | Positive deployment generation; bump after mapper corrections         | No          | `1`          |
| `TEMPORAL_PROGRESS_FILE`               | Optional file for indexed terminal-workflow progress                  | No          | —            |
| `TEMPORAL_RETENTION_PERIOD`            | Retain workflow shards for this duration                              | No          | `720h`       |
| `TEMPORAL_RETENTION_CRON`              | UTC five-field cron schedule for deleting expired workflow shards     | No          | `0 0 * * *`  |
| `TEMPORAL_DATA_WORKERS`                | Concurrent workers fetching workflow histories                        | No          | `8`          |
| `TEMPORAL_INDEX_WORKERS`               | Concurrent workers adding workflow batches to OpenSearch              | No          | `4`          |
| `TEMPORAL_INDEX_BATCH_SIZE`            | Workflows per OpenSearch bulk request                                 | No          | `100`        |

Workflow shards are named `<prefix><application-version>.<index-version>-<UTC date>`.
The application version changes with incompatible mappings; set and bump `TEMPORAL_INDEX_VERSION`
when a deployment needs to discard an incorrect mapper generation.

## License

[MIT](./LICENSE)

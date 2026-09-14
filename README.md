<div align="center">

# <img src="ui/public/favicon.svg" alt="" width="32" height="32" /> Temporal Lens

**Search engine for [Temporal.io](https://temporal.io/) workflows powered by [OpenSearch](https://github.com/opensearch-project/opensearch)**

[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?style=flat&logo=go&logoColor=white)](https://go.dev)
[![React](https://img.shields.io/badge/React-19-61DAFB?style=flat&logo=react&logoColor=black)](https://react.dev)
[![OpenSearch](https://img.shields.io/badge/OpenSearch-3.x-46398A?style=flat&logo=opensearch&logoColor=white)](https://opensearch.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)

</div>

## Features

### 🚀 Intuitive structured search

Construct complex queries with logical operators easily from the UI and search across namespaces. Queries can be constructed using:

* Workflow metadata - ID, Type, Namespace, Start/End time, Status, Search Attributes and errors.
* Activity metadata - Type/Custom activity ID, Attempts, Start/End time and errors.
* Child workflow metadata - ID, Type, Namespace, Attempts, Start/End time and errors

### 📝 Custom queries on JSON payloads

Easily create custom deployments that allow queries on custom fields extracted from JSON payloads of workflows, child workflows and activities.

Simply implement the [Mapper](https://github.com/varunbpatil/temporal-lens/blob/main/domains/workflows/ports/workflows.go#L94-L107) interface in
[mapper.go](mapper/mapper.go) and wire it in as a depdendency in [main.go](https://github.com/varunbpatil/temporal-lens/blob/main/cmd/workflows/main.go#L89).

### 🗂️ Unified view

Provides a unified view across all namespaces, even for Temporal Cloud. Queries filter across all namespaces. No more switching between namespaces in the Temporal UI.

### 📦 Bulk operations

Select individual workflows, a page of results, or all matching results to signal, reset, cancel, or terminate workflows using Temporal batch operations. Unlike the Temporal UI, this works across namespaces.

### 🤖 MCP server

Allow AI agents to do the same thing a human user would do through the UI.

## Upcoming Features

* 🔑 Authentication
* ⌨️ CLI tool and AI agent skill as an alternative to MCP server
* 👤️ Admin dashboard to manage OpenSearch indexes and much more...

## What queries are possible?

| Find                                                                | UI Filter                                                                                                  |
| ------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| Workflows whose recorded errors mention a downstream service        | `Workflow — Errors CONTAINS "inventory service"`                                                           |
| Activities whose recorded errors mention a downstream service       | `Activity — Errors CONTAINS "inventory service"`                                                           |
| Workflows with a repeatedly retried activity                        | `Activity — Attempts GTE 3`                                                                                |
| A particular activity type that started but has not finished        | `Activity — Type CONTAINS "charge-card"` AND `Activity — Finished NOT_EXISTS`                              |
| Running workflows where the pending activity has too many attempts  | `Activity — Attempts GTE 5` AND `Activity — Finished NOT_EXISTS`                                           |
| Workflows containing a paused activity                              | `Activity — Paused EQ true`                                                                                |
| Child workflows that are still open after multiple attempts         | `Child Workflow — Attempts GTE 3` AND `Child Workflow — Finished NOT_EXISTS`                               |
| [Custom Deployment] Workflows whose input payload contains ...      | `Workflow Inputs — Tenant ID CONTAINS 5d668d32-0403-45a3-8b85-d68bc3725afb`                                |
| [Custom Deployment] Activities whose output payload contains ...    | `Activity Outputs — User ID CONTAINS user_01m2gaz9p4fcca146c40d88tde`                                      |

Conditions on fields within the same activity, child workflow, or search attribute are evaluated against the same nested object. For example,
`Activity — Attempts GTE 5` AND `Activity — Finished NOT_EXISTS` returns workflows where both conditions are true for the same activity.

Custom deployments can add more fields from workflow, activity, and child-workflow JSON input/output payloads by implementing
a [Mapper](https://github.com/varunbpatil/temporal-lens/blob/main/domains/workflows/ports/workflows.go#L94-L107).
The mapper function receives a flattened JSON path like `$.response.items.0.name` and is supposed to return the canonical field,
for example `name`. A custom schema must be provided which defines the types and (optionally) supported operators of those canonical
fields thus making them filterable on the UI as `Workflow Inputs — Name CONTAINS ...`.

## Project Structure

This project uses [go-react-template](https://github.com/varunbpatil/go-react-template) as the project template
and follows the Hexagonal Architecture (Ports & Adapter pattern).

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

* [Mise](https://mise.jdx.dev) to manage the development environment.
* [Makefile](./Makefile) as the task runner.
* [Tilt](https://tilt.dev/) as the process runner.

## Usage

### Docker

```sh
# Build the image
make docker/build

# Run with environment variables from .env (copy .env.example and modify it)
make docker/run
```

### MCP server

Temporal Lens exposes a Streamable HTTP MCP endpoint at `/mcp`:

Start the service using the Docker steps above, then add the endpoint to an MCP client. For clients that use an `mcpServers` JSON configuration:

```json
{
  "mcpServers": {
    "temporal-lens": {
      "url": "http://localhost:8080/mcp"
    }
  }
}
```

The server provides search, signal, reset, cancel, and terminate actions. Set `READ_ONLY=true` in `.env` to disable those workflow mutations.

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

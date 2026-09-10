# ------------------------------------------------------------------------------
# DOCKER COMPOSE SERVICES
# ------------------------------------------------------------------------------

docker_compose("docker-compose.yml")

dc_resource("postgresql", labels=["dependencies"])
dc_resource("temporal-schema", labels=["dependencies"])
dc_resource("temporal", labels=["dependencies"])
dc_resource("temporal-create-namespace", labels=["dependencies"])
dc_resource("temporal-ui", labels=["dependencies"])
dc_resource("opensearch", labels=["dependencies"])

# ------------------------------------------------------------------------------
# READINESS CHECKS
# ------------------------------------------------------------------------------

local_resource(
    "opensearch-ready",
    cmd="make wait WAIT_FOR='http://localhost:9200/_cluster/health?wait_for_status=yellow'",
    resource_deps=["opensearch"],
    allow_parallel=True,
    labels=["readiness"],
)

local_resource(
    "temporal-ready",
    cmd="make wait WAIT_FOR=localhost:7233",
    resource_deps=["temporal-create-namespace"],
    allow_parallel=True,
    labels=["readiness"],
)

local_resource(
    "workflows-ready",
    cmd="make wait WAIT_FOR=localhost:8080",
    resource_deps=["workflows"],
    allow_parallel=True,
    labels=["readiness"],
)

# ------------------------------------------------------------------------------
# SERVICES
# ------------------------------------------------------------------------------

# Workflows service
local_resource(
    "workflows",
    serve_cmd="go run ./cmd/workflows",
    resource_deps=["opensearch-ready", "temporal-ready"],
    allow_parallel=True,
    labels=["services"],
)

# React frontend
local_resource(
    "ui",
    serve_cmd="make ui/dev",
    resource_deps=["workflows-ready"],
    allow_parallel=True,
    labels=["services"],
)

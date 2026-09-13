# ------------------------------------------------------------------------------
# DOCKER COMPOSE SERVICES
# ------------------------------------------------------------------------------

docker_compose("docker-compose.yml")

dc_resource(
    "postgresql",
    labels=["dependencies"],
)
dc_resource(
    "temporal-schema",
    labels=["dependencies"],
    resource_deps=["postgresql-ready"],
)
dc_resource(
    "temporal",
    labels=["dependencies"],
    resource_deps=["temporal-schema-ready"],
)
dc_resource(
    "temporal-create-namespace",
    labels=["dependencies"],
    resource_deps=["temporal-ready"],
)
dc_resource(
    "temporal-ui",
    labels=["dependencies"],
    resource_deps=["temporal-ready"],
)
dc_resource(
    "opensearch",
    labels=["dependencies"],
)

# ------------------------------------------------------------------------------
# LOCAL SERVICES
# ------------------------------------------------------------------------------

# Workflows service
local_resource(
    "workflows",
    serve_cmd="make go/run",
    resource_deps=["opensearch-ready", "temporal-namespace-ready"],
    allow_parallel=True,
    labels=["services"],
)

# React frontend
local_resource(
    "ui",
    serve_cmd="make ui/dev",
    links=["http://localhost:5173"],
    resource_deps=["workflows-ready"],
    allow_parallel=True,
    labels=["services"],
)

# ------------------------------------------------------------------------------
# READINESS CHECKS
# ------------------------------------------------------------------------------

local_resource(
    "postgresql-ready",
    cmd="until docker compose exec -T postgresql pg_isready -U temporal; do sleep 1; done",
    resource_deps=["postgresql"],
    allow_parallel=True,
    labels=["readiness"],
)

local_resource(
    "temporal-schema-ready",
    cmd="docker compose wait temporal-schema",
    resource_deps=["temporal-schema"],
    allow_parallel=True,
    labels=["readiness"],
)

local_resource(
    "temporal-ready",
    cmd="make wait WAIT_FOR=localhost:7233",
    resource_deps=["temporal"],
    allow_parallel=True,
    labels=["readiness"],
)

local_resource(
    "temporal-namespace-ready",
    cmd="docker compose wait temporal-create-namespace",
    resource_deps=["temporal-create-namespace"],
    allow_parallel=True,
    labels=["readiness"],
)

local_resource(
    "opensearch-ready",
    cmd="make wait WAIT_FOR='http://localhost:9200/_cluster/health?wait_for_status=yellow'",
    resource_deps=["opensearch"],
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

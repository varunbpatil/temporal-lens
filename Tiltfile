# Temporal Lens Tiltfile

# Start the docker compose services
docker_compose("docker-compose.yml")

# Workflows service
local_resource(
    "workflows",
    serve_cmd="go run ./cmd/workflows",
    resource_deps=["opensearch"],
)

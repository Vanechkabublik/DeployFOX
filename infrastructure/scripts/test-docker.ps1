$ErrorActionPreference = 'Stop'
$taskRoot = (Resolve-Path "$PSScriptRoot/../..").Path
Set-Location $taskRoot
$ErrorActionPreference = 'Continue'
$existingNetwork = docker network ls -q --filter name='^deployfox-apps$'
if (-not $existingNetwork) { docker network create deployfox-apps | Out-Null }
if ($LASTEXITCODE -ne 0) { throw 'Cannot create application network' }
$existingCaddy = docker ps -aq --filter name='^deployfox-caddy$'
if ($existingCaddy) {
  $qaLabels = docker inspect --format '{{json .Config.Labels}}' deployfox-caddy | ConvertFrom-Json
  if ($qaLabels.'deployfox.qa' -ne 'true') { throw 'deployfox-caddy already exists and is not owned by this test suite' }
} else {
  docker run -d --name deployfox-caddy --label deployfox.qa=true --network deployfox-apps -p 127.0.0.1:8088:80 -v "${taskRoot}/infrastructure/caddy/Caddyfile.local:/etc/caddy/Caddyfile:ro" -v deployfox_qa_routes:/routes:ro caddy:2-alpine | Out-Null
  if ($LASTEXITCODE -ne 0) { throw 'Cannot start test Caddy' }
}
docker run --rm --dns 1.1.1.1 -v "${taskRoot}/backend:/src" -v deployfox_gomod:/go/pkg/mod -v deployfox_gocache:/root/.cache/go-build -w /src golang:1.27.3-alpine sh -c 'CGO_ENABLED=0 go test -c ./internal/docker -o /src/agent.integration.test'
if ($LASTEXITCODE -ne 0) { throw 'Cannot compile Docker integration suite' }
$testBuilder = if ($env:DOCKER_TEST_BUILDER) { $env:DOCKER_TEST_BUILDER } else { 'deployfox' }
docker run --rm --cap-drop ALL --security-opt no-new-privileges:true --memory 512m --pids-limit 512 --network deployfox-apps -e RUN_DOCKER_TESTS=1 -e "RUN_MYSQL_TESTS=$env:RUN_MYSQL_TESTS" -e CADDY_HTTP_ONLY=1 -e "DOCKER_TEST_BUILDER=$testBuilder" -v /var/run/docker.sock:/var/run/docker.sock -v /var/lib/docker:/host/docker:ro -v deployfox_qa_routes:/routes -v deployfox_qa_work:/work -v deployfox_qa_backups:/backups -v deployfox_qa_docker:/root/.docker -v "${taskRoot}/backend/agent.integration.test:/tests:ro" --entrypoint /tests deployfox-agent:local '-test.v=true' '-test.run=TestReal' '-test.timeout=10m'
if ($LASTEXITCODE -ne 0) { throw 'Docker integration tests failed' }

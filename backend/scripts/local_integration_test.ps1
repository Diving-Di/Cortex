$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
. (Join-Path $PSScriptRoot 'local_test_env.ps1')

function Assert-NativeSuccess([string]$step) {
    if ($LASTEXITCODE -ne 0) { throw "$step failed (exit $LASTEXITCODE)" }
}

Push-Location $repoRoot
try {
    docker compose -f docker-compose.ci.yml up -d --wait db redis minio kafka elasticsearch
    Assert-NativeSuccess 'Start integration dependencies'
    docker compose -f docker-compose.ci.yml run --rm minio-init
    Assert-NativeSuccess 'Initialize private bucket'
    $topics = Invoke-RestMethod http://127.0.0.1:58082/topics
    if ($topics -notcontains 'cortex.integration.v1') {
        docker compose -f docker-compose.ci.yml run --rm kafka-init
        Assert-NativeSuccess 'Initialize Kafka topic'
    }
    & (Join-Path $PSScriptRoot 'ci_dependencies_smoke.ps1')
    Set-Location (Join-Path $repoRoot 'backend')
    go run ./cmd/migrate -steps 0 up
    Assert-NativeSuccess 'Apply migrations'
    go vet ./...
    Assert-NativeSuccess 'Go vet'
    go test -count=1 -v ./...
    Assert-NativeSuccess 'Full Go tests with PostgreSQL and Redis'
    go build ./cmd/server
    Assert-NativeSuccess 'Server build'
} finally {
    Pop-Location
}

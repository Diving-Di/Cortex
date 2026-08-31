param(
    [Parameter(Mandatory = $true)][string]$DeploymentEvidence,
    [string]$ComposeProjectDirectory = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
)

$ErrorActionPreference = "Stop"
$evidence = (Resolve-Path -LiteralPath $DeploymentEvidence).Path
$state = Get-Content -Raw -LiteralPath $evidence | ConvertFrom-Json
$services = @("backend", "frontend", "document-parser", "embedding-service", "reranker-service")
foreach ($service in $services) {
    if (-not $state.previous.$service) { throw "deployment evidence has no previous image for $service" }
}

Push-Location $ComposeProjectDirectory
try {
    $env:BACKEND_IMAGE = $state.previous.backend
    $env:FRONTEND_IMAGE = $state.previous.frontend
    $env:DOCUMENT_PARSER_IMAGE = $state.previous."document-parser"
    $env:EMBEDDING_IMAGE = $state.previous."embedding-service"
    $env:RERANKER_IMAGE = $state.previous."reranker-service"
    $immutable = @($services | Where-Object { $state.previous.$_ -match '@sha256:' })
    if ($immutable.Count -eq $services.Count) {
        docker compose pull $services
        if ($LASTEXITCODE -ne 0) { throw "previous image pull failed" }
    }
    docker compose up -d --no-build --wait $services
    if ($LASTEXITCODE -ne 0) { throw "rollback services did not become healthy" }
    & (Join-Path $PSScriptRoot "production_acceptance.ps1") -ComposeProjectDirectory $ComposeProjectDirectory
} finally {
    Remove-Item Env:BACKEND_IMAGE -ErrorAction SilentlyContinue
    Remove-Item Env:FRONTEND_IMAGE -ErrorAction SilentlyContinue
    Remove-Item Env:DOCUMENT_PARSER_IMAGE -ErrorAction SilentlyContinue
    Remove-Item Env:EMBEDDING_IMAGE -ErrorAction SilentlyContinue
    Remove-Item Env:RERANKER_IMAGE -ErrorAction SilentlyContinue
    Pop-Location
}

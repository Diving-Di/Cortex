param(
    [Parameter(Mandatory = $true)][string]$DeploymentEvidence,
    [string]$ComposeProjectDirectory = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
)

$ErrorActionPreference = "Stop"
$evidence = (Resolve-Path -LiteralPath $DeploymentEvidence).Path
$state = Get-Content -Raw -LiteralPath $evidence | ConvertFrom-Json
if (-not $state.previous.backend -or -not $state.previous.frontend) { throw "deployment evidence has no previous image pair" }

Push-Location $ComposeProjectDirectory
try {
    $env:BACKEND_IMAGE = $state.previous.backend
    $env:FRONTEND_IMAGE = $state.previous.frontend
    if ($state.previous.backend -match '@sha256:' -and $state.previous.frontend -match '@sha256:') {
        docker compose pull backend frontend
        if ($LASTEXITCODE -ne 0) { throw "previous image pull failed" }
    }
    docker compose up -d --no-build --wait backend frontend
    if ($LASTEXITCODE -ne 0) { throw "rollback services did not become healthy" }
    & (Join-Path $PSScriptRoot "production_acceptance.ps1") -ComposeProjectDirectory $ComposeProjectDirectory
} finally {
    Remove-Item Env:BACKEND_IMAGE -ErrorAction SilentlyContinue
    Remove-Item Env:FRONTEND_IMAGE -ErrorAction SilentlyContinue
    Pop-Location
}

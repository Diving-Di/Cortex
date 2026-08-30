param(
    [Parameter(Mandatory = $true)][string]$BackendImage,
    [Parameter(Mandatory = $true)][string]$FrontendImage,
    [string]$ComposeProjectDirectory = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path,
    [string]$EvidenceDirectory = (Join-Path (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path ".runtime\releases")
)

$ErrorActionPreference = "Stop"
foreach ($image in @($BackendImage, $FrontendImage)) {
    if ($image -notmatch '^[^\s]+@sha256:[a-f0-9]{64}$') { throw "release images must be immutable digest references" }
}

$releaseID = [DateTimeOffset]::UtcNow.ToString("yyyyMMddTHHmmssZ")
$evidence = Join-Path ([IO.Path]::GetFullPath($EvidenceDirectory)) $releaseID
New-Item -ItemType Directory -Path $evidence -Force | Out-Null
$backupDirectory = Join-Path $evidence "backup"
$state = $null
$previousBackend = $null
$previousFrontend = $null

Push-Location $ComposeProjectDirectory
try {
    $previousBackendContainer = docker compose ps -q backend
    $previousFrontendContainer = docker compose ps -q frontend
    if (-not $previousBackendContainer -or -not $previousFrontendContainer) { throw "backend and frontend must be running before deployment" }
    $previousBackend = (docker inspect $previousBackendContainer --format '{{.Config.Image}}').Trim()
    $previousFrontend = (docker inspect $previousFrontendContainer --format '{{.Config.Image}}').Trim()
    $state = [ordered]@{
        release_id = $releaseID
        started_at_utc = [DateTimeOffset]::UtcNow.ToString("o")
        previous = @{ backend = $previousBackend; frontend = $previousFrontend }
        target = @{ backend = $BackendImage; frontend = $FrontendImage }
        status = "deploying"
    }
    $state | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $evidence "deployment.json") -Encoding utf8NoBOM

    & (Join-Path $PSScriptRoot "backup.ps1") -OutputDirectory $backupDirectory | Set-Content -LiteralPath (Join-Path $evidence "backup-result.json") -Encoding utf8NoBOM

    $env:BACKEND_IMAGE = $BackendImage
    $env:FRONTEND_IMAGE = $FrontendImage
    docker compose pull backend frontend
    if ($LASTEXITCODE -ne 0) { throw "release image pull failed" }
    docker compose run --rm --no-deps --entrypoint /app/migrate backend -steps 0 up
    if ($LASTEXITCODE -ne 0) { throw "forward migration failed" }
    docker compose up -d --no-build --wait backend frontend
    if ($LASTEXITCODE -ne 0) { throw "release services did not become healthy" }
    & (Join-Path $PSScriptRoot "production_acceptance.ps1") -ComposeProjectDirectory $ComposeProjectDirectory | Set-Content -LiteralPath (Join-Path $evidence "acceptance.json") -Encoding utf8NoBOM

    $state["status"] = "success"
    $state["completed_at_utc"] = [DateTimeOffset]::UtcNow.ToString("o")
    $state | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $evidence "deployment.json") -Encoding utf8NoBOM
    $state | ConvertTo-Json -Depth 5
} catch {
    $failure = $_
    if ($previousBackend -and $previousFrontend) {
        $env:BACKEND_IMAGE = $previousBackend
        $env:FRONTEND_IMAGE = $previousFrontend
        docker compose up -d --no-build --wait backend frontend 2>&1 | Set-Content -LiteralPath (Join-Path $evidence "automatic-rollback.log") -Encoding utf8NoBOM
        $rollbackSucceeded = $LASTEXITCODE -eq 0
    }
    if ($state) {
        $state["status"] = if ($rollbackSucceeded) { "rolled_back" } else { "rollback_failed" }
        $state["failed_at_utc"] = [DateTimeOffset]::UtcNow.ToString("o")
        $state["failure"] = $failure.Exception.Message
        $state | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $evidence "deployment.json") -Encoding utf8NoBOM
    }
    throw
} finally {
    Remove-Item Env:BACKEND_IMAGE -ErrorAction SilentlyContinue
    Remove-Item Env:FRONTEND_IMAGE -ErrorAction SilentlyContinue
    Pop-Location
}

param(
    [Parameter(Mandatory = $true)][string]$BackendImage,
    [Parameter(Mandatory = $true)][string]$FrontendImage,
    [Parameter(Mandatory = $true)][string]$DocumentParserImage,
    [Parameter(Mandatory = $true)][string]$EmbeddingImage,
    [Parameter(Mandatory = $true)][string]$RerankerImage,
    [string]$ComposeProjectDirectory = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path,
    [string]$EvidenceDirectory = (Join-Path (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path ".runtime\releases")
)

$ErrorActionPreference = "Stop"
$targetImages = [ordered]@{
    backend = $BackendImage
    frontend = $FrontendImage
    "document-parser" = $DocumentParserImage
    "embedding-service" = $EmbeddingImage
    "reranker-service" = $RerankerImage
}
$releaseServices = @($targetImages.Keys)
foreach ($image in $targetImages.Values) {
    if ($image -notmatch '^[^\s]+@sha256:[a-f0-9]{64}$') { throw "release images must be immutable digest references" }
}

$releaseID = [DateTimeOffset]::UtcNow.ToString("yyyyMMddTHHmmssZ")
$evidence = Join-Path ([IO.Path]::GetFullPath($EvidenceDirectory)) $releaseID
New-Item -ItemType Directory -Path $evidence -Force | Out-Null
$backupDirectory = Join-Path $evidence "backup"
$state = $null
$previousImages = [ordered]@{}

Push-Location $ComposeProjectDirectory
try {
    foreach ($service in $targetImages.Keys) {
        $container = docker compose ps -q $service
        if (-not $container) { throw "$service must be running before deployment" }
        $previousImages[$service] = (docker inspect $container --format '{{.Config.Image}}').Trim()
    }
    $state = [ordered]@{
        release_id = $releaseID
        started_at_utc = [DateTimeOffset]::UtcNow.ToString("o")
        previous = $previousImages
        target = $targetImages
        status = "deploying"
    }
    $state | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $evidence "deployment.json") -Encoding utf8NoBOM

    & (Join-Path $PSScriptRoot "backup.ps1") -OutputDirectory $backupDirectory | Set-Content -LiteralPath (Join-Path $evidence "backup-result.json") -Encoding utf8NoBOM

    $env:BACKEND_IMAGE = $BackendImage
    $env:FRONTEND_IMAGE = $FrontendImage
    $env:DOCUMENT_PARSER_IMAGE = $DocumentParserImage
    $env:EMBEDDING_IMAGE = $EmbeddingImage
    $env:RERANKER_IMAGE = $RerankerImage
    docker compose pull $releaseServices
    if ($LASTEXITCODE -ne 0) { throw "release image pull failed" }
    docker compose run --rm --no-deps --entrypoint /app/migrate backend -steps 0 up
    if ($LASTEXITCODE -ne 0) { throw "forward migration failed" }
    docker compose up -d --no-build --wait $releaseServices
    if ($LASTEXITCODE -ne 0) { throw "release services did not become healthy" }
    & (Join-Path $PSScriptRoot "production_acceptance.ps1") -ComposeProjectDirectory $ComposeProjectDirectory | Set-Content -LiteralPath (Join-Path $evidence "acceptance.json") -Encoding utf8NoBOM

    $state["status"] = "success"
    $state["completed_at_utc"] = [DateTimeOffset]::UtcNow.ToString("o")
    $state | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $evidence "deployment.json") -Encoding utf8NoBOM
    $state | ConvertTo-Json -Depth 5
} catch {
    $failure = $_
    if ($previousImages.Count -eq $targetImages.Count) {
        $env:BACKEND_IMAGE = $previousImages.backend
        $env:FRONTEND_IMAGE = $previousImages.frontend
        $env:DOCUMENT_PARSER_IMAGE = $previousImages."document-parser"
        $env:EMBEDDING_IMAGE = $previousImages."embedding-service"
        $env:RERANKER_IMAGE = $previousImages."reranker-service"
        docker compose up -d --no-build --wait $releaseServices 2>&1 | Set-Content -LiteralPath (Join-Path $evidence "automatic-rollback.log") -Encoding utf8NoBOM
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
    Remove-Item Env:DOCUMENT_PARSER_IMAGE -ErrorAction SilentlyContinue
    Remove-Item Env:EMBEDDING_IMAGE -ErrorAction SilentlyContinue
    Remove-Item Env:RERANKER_IMAGE -ErrorAction SilentlyContinue
    Pop-Location
}

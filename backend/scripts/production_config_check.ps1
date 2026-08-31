param(
    [string]$ComposeProjectDirectory = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path,
    [string]$ProductionComposeFile = "docker-compose.production.yml"
)

$ErrorActionPreference = "Stop"
$requiredImages = @("backend", "frontend", "document-parser", "embedding-service", "reranker-service")

Push-Location $ComposeProjectDirectory
try {
    $configJSON = docker compose -f docker-compose.yml -f $ProductionComposeFile config --format json
    if ($LASTEXITCODE -ne 0 -or -not $configJSON) { throw "production Compose configuration is invalid" }
    $config = $configJSON | ConvertFrom-Json

    foreach ($service in $requiredImages) {
        $image = [string]$config.services.$service.image
        if ($image -notmatch '^[^\s]+@sha256:[a-f0-9]{64}$') {
            throw "$service must use an immutable sha256 image digest"
        }
        if ($config.services.$service.build) { throw "$service must not have a production build context" }
    }
    $backendEnvironment = @{}
    foreach ($entry in @($config.services.backend.environment)) {
        if ($entry -is [string] -and $entry -match '^([^=]+)=(.*)$') { $backendEnvironment[$Matches[1]] = $Matches[2] }
    }
    if ($config.services.backend.environment -is [pscustomobject]) {
        $config.services.backend.environment.psobject.Properties | ForEach-Object { $backendEnvironment[$_.Name] = [string]$_.Value }
    }
    if ($backendEnvironment.APP_ENV -ne "production") { throw "APP_ENV must be production" }
    if ($backendEnvironment.PUBLIC_BASE_URL -notmatch '^https://') { throw "PUBLIC_BASE_URL must use https" }
    if ($backendEnvironment.CORS_ORIGINS -notmatch '^https://') { throw "CORS_ORIGINS must use https" }
    if ($env:ALERTMANAGER_CONFIG_FILE -match 'deploy[/\\]alertmanager[/\\]alertmanager.yml$') {
        throw "the local-only Alertmanager configuration is forbidden in production"
    }
    [pscustomobject]@{ Status = "passed"; ImmutableImages = $requiredImages.Count; PublicURL = $backendEnvironment.PUBLIC_BASE_URL } | ConvertTo-Json -Compress
}
finally {
    Pop-Location
}

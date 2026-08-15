param(
    [string]$Sizes = "100,1000,10000",
    [int]$Queries = 100,
    [string]$OutputFile
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$backendPath = Join-Path $repoRoot "backend"
$artifactPath = Join-Path $repoRoot "artifacts"
New-Item -ItemType Directory -Path $artifactPath -Force | Out-Null
if (-not $OutputFile) { $OutputFile = Join-Path $artifactPath ("capacity-" + (Get-Date -Format "yyyyMMdd-HHmmss") + ".json") }
$output = [IO.Path]::GetFullPath($OutputFile)
if ([IO.Path]::GetDirectoryName($output) -ne [IO.Path]::GetFullPath($artifactPath)) { throw "capacity output must be directly under artifacts" }

$db = docker compose -f (Join-Path $repoRoot "docker-compose.yml") -p cortex ps -q db
if (-not $db) { throw "database container is not running" }
$migrationPassword = docker exec $db printenv POSTGRES_PASSWORD
$appPasswordLine = Get-Content (Join-Path $repoRoot ".env") | Where-Object { $_ -match '^POSTGRES_APP_PASSWORD=' } | Select-Object -First 1
if (-not $appPasswordLine) { throw "POSTGRES_APP_PASSWORD is not configured" }
$appPassword = $appPasswordLine.Substring("POSTGRES_APP_PASSWORD=".Length)
$commit = (git -C $repoRoot rev-parse HEAD).Trim()
$dirty = if (git -C $repoRoot status --porcelain) { "true" } else { "false" }
$outputName = [IO.Path]::GetFileName($output)

docker run --rm --network cortex_default `
    -e "DATABASE_URL=postgresql://cortex_app:$appPassword@db:5432/cortex" `
    -e "MIGRATION_DATABASE_URL=postgresql://cortex_migrator:$migrationPassword@db:5432/cortex" `
    -e "CAPACITY_GIT_COMMIT=$commit" -e "CAPACITY_GIT_DIRTY=$dirty" `
    --mount "type=bind,source=$backendPath,target=/src" `
    --mount "type=bind,source=$artifactPath,target=/artifacts" `
    -w /src golang:1.26-alpine sh -c "go run ./cmd/capacity-check -sizes '$Sizes' -queries $Queries > '/artifacts/$outputName'"
if ($LASTEXITCODE -ne 0) { throw "capacity check failed" }
Get-Content -Raw -LiteralPath $output

param(
    [Parameter(Mandatory = $true)][string]$OutputDirectory,
    [string]$ComposeProject = "cortex",
    [string]$ComposeFile,
    [string]$DatabaseVolume = "cortex_db_data_v2",
    [string]$AppDataVolume = "cortex_app_data_v2",
    [string]$MinIODataVolume = "cortex_minio_data",
    [string]$MetricsVolume = "cortex_metrics_data"
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$target = [IO.Path]::GetFullPath($OutputDirectory)
$root = [IO.Path]::GetPathRoot($target)
if ($target -eq $root -or $target -eq $repoRoot) { throw "backup output must be a dedicated directory" }
if (Test-Path -LiteralPath $target) {
    if (@(Get-ChildItem -LiteralPath $target -Force).Count -ne 0) { throw "backup output directory must be empty" }
} else {
    New-Item -ItemType Directory -Path $target | Out-Null
}

$composeArgs = @()
if ($ComposeFile) {
    $composeArgs += @("-f", (Resolve-Path -LiteralPath $ComposeFile).Path)
}
$dbContainer = docker compose @composeArgs -p $ComposeProject ps -q db
if (-not $dbContainer) { throw "source database container is not running" }
$network = docker inspect $dbContainer --format '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}}{{end}}'
$dbAlias = docker inspect $dbContainer --format '{{.Name}}'
$dbAlias = $dbAlias.TrimStart('/')
$dbPassword = docker exec $dbContainer printenv POSTGRES_PASSWORD
if (-not $dbPassword) { throw "source migration password is unavailable" }
$started = [DateTimeOffset]::UtcNow
$timer = [Diagnostics.Stopwatch]::StartNew()

docker run --rm --network $network -e "PGPASSWORD=$dbPassword" `
    --mount "type=bind,source=$target,target=/backup" postgres:16.12-bookworm `
    pg_dump -h $dbAlias -U cortex_migrator -d cortex -Fc --no-owner --no-privileges -f /backup/database.dump
if ($LASTEXITCODE -ne 0) { throw "database backup failed" }

$pathList = Join-Path $target ".referenced-paths.txt"
try {
    New-Item -ItemType File -Path $pathList -Force | Out-Null
    $referencedPaths = @(docker exec -e "PGPASSWORD=$dbPassword" $dbContainer psql -U cortex_migrator -d cortex -At `
        -c "SELECT stored_path FROM attachments WHERE storage_backend='local' UNION SELECT stored_path FROM knowledge_documents WHERE storage_backend='local' AND stored_path IS NOT NULL UNION SELECT stored_path FROM knowledge_assets WHERE storage_backend='local' UNION SELECT storage_path FROM research_assets ORDER BY 1")
    if ($LASTEXITCODE -ne 0) { throw "application data reference query failed" }
    if ($referencedPaths.Count -gt 0) {
        $referencedPaths | Set-Content -LiteralPath $pathList -Encoding utf8NoBOM
    }
    docker run --rm --mount "type=volume,source=$AppDataVolume,target=/data,readonly" `
        --mount "type=bind,source=$target,target=/backup" alpine:3.23 `
        sh -c 'sed -i "s/\r$//" /backup/.referenced-paths.txt; missing=0; while IFS= read -r p; do [ -z "$p" ] || [ -f "/data/$p" ] || missing=$((missing+1)); done < /backup/.referenced-paths.txt; [ "$missing" -eq 0 ] || exit 42; if [ -s /backup/.referenced-paths.txt ]; then tar -czf /backup/app-data.tar.gz -C /data -T /backup/.referenced-paths.txt; else mkdir -p /tmp/empty; tar -czf /backup/app-data.tar.gz -C /tmp/empty .; fi'
    if ($LASTEXITCODE -eq 42) { throw "database references missing application files" }
    if ($LASTEXITCODE -ne 0) { throw "application data backup failed" }
} finally {
    Remove-Item -LiteralPath $pathList -Force -ErrorAction SilentlyContinue
}

docker run --rm --mount "type=volume,source=$MinIODataVolume,target=/minio,readonly" `
    --mount "type=bind,source=$target,target=/backup" alpine:3.23 `
    tar -czf /backup/minio-data.tar.gz -C /minio .
if ($LASTEXITCODE -ne 0) { throw "MinIO data backup failed" }

$snapshot = docker exec -e "PGPASSWORD=$dbPassword" $dbContainer psql -U cortex_migrator -d cortex -At `
    -c "SELECT clock_timestamp() AT TIME ZONE 'UTC',COALESCE(max(version),0),(SELECT count(*) FROM pg_tables WHERE schemaname='public') FROM schema_migrations"
if ($LASTEXITCODE -ne 0 -or -not $snapshot) { throw "backup metadata query failed" }
$parts = $snapshot.Trim().Split('|')
$gitCommit = (git -C $repoRoot rev-parse HEAD).Trim()
$gitDirty = [bool](git -C $repoRoot status --porcelain)
$dbFile = Get-Item (Join-Path $target "database.dump")
$appFile = Get-Item (Join-Path $target "app-data.tar.gz")
$minioFile = Get-Item (Join-Path $target "minio-data.tar.gz")
$manifest = [ordered]@{
    format_version = 1
    created_at_utc = $started.ToString("o")
    database_snapshot_utc = $parts[0]
    git_commit = $gitCommit
    git_dirty = $gitDirty
    migration_version = [int]$parts[1]
    public_table_count = [int]$parts[2]
    database = @{ file = $dbFile.Name; bytes = $dbFile.Length; sha256 = (Get-FileHash $dbFile -Algorithm SHA256).Hash.ToLowerInvariant() }
    app_data = @{ file = $appFile.Name; bytes = $appFile.Length; sha256 = (Get-FileHash $appFile -Algorithm SHA256).Hash.ToLowerInvariant() }
    minio_data = @{ file = $minioFile.Name; bytes = $minioFile.Length; sha256 = (Get-FileHash $minioFile -Algorithm SHA256).Hash.ToLowerInvariant() }
    source = @{ database_volume = $DatabaseVolume; app_data_volume = $AppDataVolume; minio_data_volume = $MinIODataVolume }
}
$timer.Stop()
$manifest.backup_duration_seconds = [Math]::Round($timer.Elapsed.TotalSeconds, 3)
$manifest | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath (Join-Path $target "manifest.json") -Encoding utf8NoBOM
$successUnixTime = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
docker run --rm --mount "type=volume,source=$MetricsVolume,target=/metrics" alpine:3.23 `
    sh -c "printf 'cortex_backup_last_success_unixtime $successUnixTime\ncortex_backup_duration_seconds $($manifest.backup_duration_seconds)\n' > /metrics/cortex_backup.prom.tmp && mv /metrics/cortex_backup.prom.tmp /metrics/cortex_backup.prom"
if ($LASTEXITCODE -ne 0) { throw "backup succeeded but metrics publication failed" }
[pscustomobject]@{
    BackupDirectory = $target
    MigrationVersion = $manifest.migration_version
    PublicTables = $manifest.public_table_count
    DatabaseBytes = $dbFile.Length
    AppDataBytes = $appFile.Length
    MinIODataBytes = $minioFile.Length
    DurationSeconds = $manifest.backup_duration_seconds
} | ConvertTo-Json -Compress

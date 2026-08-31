param(
    [Parameter(Mandatory = $true)][string]$BackupDirectory,
    [switch]$KeepEnvironment,
    [switch]$SkipSmoke,
    [string]$BackendImage = $(if ($env:BACKEND_IMAGE) { $env:BACKEND_IMAGE } else { "cortex-backend" }),
    [string]$MetricsVolume = "cortex_metrics_data"
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$backup = (Resolve-Path -LiteralPath $BackupDirectory).Path
$manifestPath = Join-Path $backup "manifest.json"
if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) { throw "manifest.json is missing" }
$manifest = Get-Content -Raw -LiteralPath $manifestPath | ConvertFrom-Json
if ($manifest.format_version -ne 1) { throw "unsupported backup format" }
docker image inspect $BackendImage 2>$null | Out-Null
if ($LASTEXITCODE -ne 0) { throw "restore backend image is not available: $BackendImage" }
$assets = @($manifest.database, $manifest.app_data)
if ($manifest.minio_data) { $assets += $manifest.minio_data }
foreach ($asset in $assets) {
    $path = Join-Path $backup $asset.file
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "backup asset is missing" }
    if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $asset.sha256) { throw "backup checksum mismatch" }
}

$suffix = [guid]::NewGuid().ToString("N").Substring(0, 10)
$prefix = "cortex-restore-$suffix"
$network = "$prefix-net"
$dbVolume = "$prefix-db"
$appVolume = "$prefix-app"
$minioVolume = "$prefix-minio"
$dbContainer = "$prefix-db"
$backendContainer = "$prefix-backend"
$minioContainer = "$prefix-minio"
$dbPassword = [guid]::NewGuid().ToString("N")
$appPassword = [guid]::NewGuid().ToString("N")
$minioUser = "restoreadmin"
$minioPassword = ([guid]::NewGuid().ToString("N") + [guid]::NewGuid().ToString("N"))
$report = [ordered]@{ environment = $prefix; started_at_utc = [DateTimeOffset]::UtcNow.ToString("o") }
$total = [Diagnostics.Stopwatch]::StartNew()

function Remove-IsolatedEnvironment {
    docker rm -f $backendContainer $minioContainer $dbContainer 2>$null | Out-Null
    docker network rm $network 2>$null | Out-Null
    docker volume rm $dbVolume $appVolume $minioVolume 2>$null | Out-Null
}

try {
    docker network create $network | Out-Null
    docker volume create $dbVolume | Out-Null
    docker volume create $appVolume | Out-Null
    docker volume create $minioVolume | Out-Null
    docker run -d --name $dbContainer --network $network --network-alias db `
        -e POSTGRES_DB=cortex -e POSTGRES_USER=cortex_migrator -e "POSTGRES_PASSWORD=$dbPassword" `
        --mount "type=volume,source=$dbVolume,target=/var/lib/postgresql/data" `
        pgvector/pgvector:0.8.1-pg16-bookworm | Out-Null
    $ready = $false
    foreach ($attempt in 1..60) {
        docker exec -e "PGPASSWORD=$dbPassword" $dbContainer pg_isready -U cortex_migrator -d cortex 2>$null | Out-Null
        if ($LASTEXITCODE -eq 0) { $ready = $true; break }
        Start-Sleep -Seconds 1
    }
    if (-not $ready) { throw "isolated database did not become ready" }

    $restoreTimer = [Diagnostics.Stopwatch]::StartNew()
    docker run --rm --network $network -e "PGPASSWORD=$dbPassword" `
        --mount "type=bind,source=$backup,target=/backup,readonly" postgres:16.12-bookworm `
        pg_restore -h db -U cortex_migrator -d cortex --exit-on-error --no-owner --no-privileges "/backup/$($manifest.database.file)"
    if ($LASTEXITCODE -ne 0) { throw "database restore failed" }
    docker exec -e "PGPASSWORD=$dbPassword" $dbContainer psql -v ON_ERROR_STOP=1 -U cortex_migrator -d cortex `
        -c "CREATE ROLE cortex_app LOGIN PASSWORD '$appPassword'; GRANT CONNECT ON DATABASE cortex TO cortex_app; GRANT USAGE ON SCHEMA public TO cortex_app; GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO cortex_app; GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO cortex_app;"
    if ($LASTEXITCODE -ne 0) { throw "low-privilege role creation failed" }
    $restoreTimer.Stop()
    $report.database_restore_seconds = [Math]::Round($restoreTimer.Elapsed.TotalSeconds, 3)

    $dataTimer = [Diagnostics.Stopwatch]::StartNew()
    docker run --rm --mount "type=volume,source=$appVolume,target=/data" `
        --mount "type=bind,source=$backup,target=/backup,readonly" alpine:3.23 `
        tar -xzf "/backup/$($manifest.app_data.file)" -C /data
    if ($LASTEXITCODE -ne 0) { throw "application data restore failed" }
    $dataTimer.Stop()
    $report.app_data_restore_seconds = [Math]::Round($dataTimer.Elapsed.TotalSeconds, 3)

    if ($manifest.minio_data) {
        $minioTimer = [Diagnostics.Stopwatch]::StartNew()
        docker run --rm --mount "type=volume,source=$minioVolume,target=/data" `
            --mount "type=bind,source=$backup,target=/backup,readonly" alpine:3.23 `
            tar -xzf "/backup/$($manifest.minio_data.file)" -C /data
        if ($LASTEXITCODE -ne 0) { throw "MinIO data restore failed" }
        docker run -d --name $minioContainer --network $network --network-alias minio `
            -e "MINIO_ROOT_USER=$minioUser" -e "MINIO_ROOT_PASSWORD=$minioPassword" `
            --mount "type=volume,source=$minioVolume,target=/data" `
            quay.io/minio/minio:RELEASE.2025-07-23T15-54-02Z server /data | Out-Null
        $minioReady = $false
        foreach ($attempt in 1..60) {
            docker run --rm --network $network quay.io/minio/mc:RELEASE.2025-07-21T05-28-08Z `
                alias set restored http://minio:9000 $minioUser $minioPassword 2>$null | Out-Null
            if ($LASTEXITCODE -eq 0) { $minioReady = $true; break }
            Start-Sleep -Seconds 1
        }
        if (-not $minioReady) { throw "isolated MinIO did not become ready" }
        $minioTimer.Stop()
        $report.minio_restore_seconds = [Math]::Round($minioTimer.Elapsed.TotalSeconds, 3)
    }

    $migrationTimer = [Diagnostics.Stopwatch]::StartNew()
    docker run --rm --network $network --entrypoint /app/migrate `
        -e "MIGRATION_DATABASE_URL=postgresql://cortex_migrator:$dbPassword@db:5432/cortex" `
        $BackendImage -steps 0 up
    if ($LASTEXITCODE -ne 0) { throw "migration failed" }
    $migrationTimer.Stop()
    $report.migration_seconds = [Math]::Round($migrationTimer.Elapsed.TotalSeconds, 3)

    $checkTimer = [Diagnostics.Stopwatch]::StartNew()
    $checks = docker exec -e "PGPASSWORD=$dbPassword" $dbContainer psql -U cortex_migrator -d cortex -At `
        -c "SELECT (SELECT count(*) FROM pg_tables WHERE schemaname='public'),COALESCE((SELECT max(version) FROM schema_migrations),0),(SELECT count(*) FROM pg_class WHERE relnamespace='public'::regnamespace AND relkind='r' AND relrowsecurity),(SELECT count(*) FROM pg_class WHERE relnamespace='public'::regnamespace AND relkind='r' AND relforcerowsecurity);"
    if ($LASTEXITCODE -ne 0) { throw "schema verification failed" }
    $values = $checks.Trim().Split('|')
    if ([int]$values[0] -lt [int]$manifest.public_table_count -or [int]$values[1] -lt [int]$manifest.migration_version -or [int]$values[2] -eq 0 -or [int]$values[3] -eq 0) { throw "schema, migration, or RLS verification failed" }
    docker run --rm --network $network -e "PGPASSWORD=$appPassword" postgres:16.12-bookworm `
        psql -h db -U cortex_app -d cortex -v ON_ERROR_STOP=1 -At -c "SELECT count(*) FROM notes" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "low-privilege connection verification failed" }

    if ($manifest.minio_data) {
        $objectKeysFile = Join-Path ([IO.Path]::GetTempPath()) "$prefix-object-keys.txt"
        try {
            New-Item -ItemType File -Path $objectKeysFile -Force | Out-Null
            $objectKeys = @(docker exec -e "PGPASSWORD=$dbPassword" $dbContainer psql -U cortex_migrator -d cortex -At `
                -c "SELECT object_key FROM attachments WHERE storage_backend='minio' AND object_key IS NOT NULL UNION SELECT object_key FROM knowledge_documents WHERE storage_backend='minio' AND object_key IS NOT NULL UNION SELECT object_key FROM knowledge_assets WHERE storage_backend='minio' AND object_key IS NOT NULL ORDER BY 1")
            if ($LASTEXITCODE -ne 0) { throw "MinIO object reference query failed" }
            if ($objectKeys.Count -gt 0) {
                $objectKeys | Set-Content -LiteralPath $objectKeysFile -Encoding utf8NoBOM
            }
            docker run --rm --network $network --entrypoint /bin/sh `
                --mount "type=bind,source=$objectKeysFile,target=/object-keys.txt,readonly" `
                --env "MINIO_USER=$minioUser" --env "MINIO_PASSWORD=$minioPassword" `
                quay.io/minio/mc:RELEASE.2025-07-21T05-28-08Z -c `
                'mc alias set restored http://minio:9000 "$MINIO_USER" "$MINIO_PASSWORD" >/dev/null || exit 41; cr=$(printf "\r"); while IFS= read -r key; do key=${key%"$cr"}; [ -z "$key" ] || mc stat "restored/cortex-private/$key" >/dev/null || exit 42; done < /object-keys.txt'
            if ($LASTEXITCODE -eq 42) { throw "database references missing MinIO objects" }
            if ($LASTEXITCODE -ne 0) { throw "MinIO object consistency verification failed" }
            $report.minio_objects_verified = @(Get-Content -LiteralPath $objectKeysFile | Where-Object { $_ }).Count
        } finally {
            Remove-Item -LiteralPath $objectKeysFile -Force -ErrorAction SilentlyContinue
        }
    }

    $pathsFile = Join-Path ([IO.Path]::GetTempPath()) "$prefix-paths.txt"
    try {
        New-Item -ItemType File -Path $pathsFile -Force | Out-Null
        $referencedPaths = @(docker exec -e "PGPASSWORD=$dbPassword" $dbContainer psql -U cortex_migrator -d cortex -At `
            -c "SELECT stored_path FROM attachments WHERE storage_backend='local' UNION SELECT stored_path FROM knowledge_documents WHERE storage_backend='local' AND stored_path IS NOT NULL UNION SELECT stored_path FROM knowledge_assets WHERE storage_backend='local'")
        if ($LASTEXITCODE -ne 0) { throw "application data reference query failed" }
        if ($referencedPaths.Count -gt 0) {
            $referencedPaths | Set-Content -LiteralPath $pathsFile -Encoding utf8NoBOM
        }
        $counts = docker run --rm --mount "type=volume,source=$appVolume,target=/data,readonly" `
            --mount "type=bind,source=$pathsFile,target=/paths.txt,readonly" alpine:3.23 `
            sh -c 'sed "s/\r$//" /paths.txt | sed "/^$/d" | sort -u >/tmp/refs; find /data/attachments /data/knowledge -type f 2>/dev/null | sed "s#^/data/##" | sort -u >/tmp/files; missing=$(comm -23 /tmp/refs /tmp/files | wc -l); orphan=$(comm -13 /tmp/refs /tmp/files | wc -l); echo "$missing|$orphan"'
        $pathCounts = $counts.Trim().Split('|')
        if ([int]$pathCounts[0] -ne 0) { throw "database references missing application files" }
        if ([int]$pathCounts[1] -ne 0) { throw "application data contains unreferenced files" }
    } finally {
        Remove-Item -LiteralPath $pathsFile -Force -ErrorAction SilentlyContinue
    }
    $checkTimer.Stop()
    $report.consistency_check_seconds = [Math]::Round($checkTimer.Elapsed.TotalSeconds, 3)
    $report.public_tables = [int]$values[0]
    $report.migration_version = [int]$values[1]
    $report.rls_tables = [int]$values[2]
    $report.force_rls_tables = [int]$values[3]
    $report.missing_files = 0
    $report.orphan_files = 0

    if (-not $SkipSmoke) {
        docker run -d --name $backendContainer --network $network -P `
            -e "DATABASE_URL=postgresql://cortex_app:$appPassword@db:5432/cortex" `
            -e "MIGRATION_DATABASE_URL=postgresql://cortex_migrator:$dbPassword@db:5432/cortex" `
            -e CORTEX_DATA_DIR=/app/data -e SCHEDULED_REPORTS_ENABLED=false `
            -e REDIS_URL=redis://invalid:6379/0 --mount "type=volume,source=$appVolume,target=/app/data" $BackendImage | Out-Null
        $port = $null
        foreach ($attempt in 1..60) {
            $mapping = docker port $backendContainer 8000/tcp 2>$null
            if ($mapping -match ':(\d+)$') {
                $port = $matches[1]
                try { Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$port/readyz" | Out-Null; break } catch { }
            }
            Start-Sleep -Seconds 1
        }
        if (-not $port) { throw "isolated backend did not publish a port" }
        $smokeTimer = [Diagnostics.Stopwatch]::StartNew()
        $env:GO_BACKEND_URL = "http://127.0.0.1:$port"
        try { & (Join-Path $PSScriptRoot "non_ai_smoke.ps1") | Out-Null } finally { Remove-Item Env:GO_BACKEND_URL -ErrorAction SilentlyContinue }
        $smokeTimer.Stop()
        $report.non_ai_smoke_seconds = [Math]::Round($smokeTimer.Elapsed.TotalSeconds, 3)
    }
    $total.Stop()
    $report.completed_at_utc = [DateTimeOffset]::UtcNow.ToString("o")
    $report.rto_seconds = [Math]::Round($total.Elapsed.TotalSeconds, 3)
    $snapshotTime = [DateTimeOffset]::Parse(($manifest.database_snapshot_utc + "Z").Replace("+00Z", "+00:00"))
    $report.observed_rpo_seconds = [Math]::Max(0, [Math]::Round(([DateTimeOffset]::Parse($report.started_at_utc) - $snapshotTime).TotalSeconds, 3))
    $report.status = "success"
    $successUnixTime = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
    docker run --rm --mount "type=volume,source=$MetricsVolume,target=/metrics" alpine:3.23 `
        sh -c "printf 'cortex_restore_drill_last_success_unixtime $successUnixTime\ncortex_restore_drill_rto_seconds $($report.rto_seconds)\ncortex_restore_drill_observed_rpo_seconds $($report.observed_rpo_seconds)\n' > /metrics/cortex_restore.prom.tmp && mv /metrics/cortex_restore.prom.tmp /metrics/cortex_restore.prom"
    if ($LASTEXITCODE -ne 0) { throw "restore drill succeeded but metrics publication failed" }
    $report | ConvertTo-Json -Depth 5
} finally {
    if (-not $KeepEnvironment) { Remove-IsolatedEnvironment }
}

param(
    [string]$ComposeProjectDirectory = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path,
    [int]$LogTail = 1000
)

$ErrorActionPreference = "Stop"
Push-Location $ComposeProjectDirectory
try {
    docker compose config --quiet
    $required = @("db", "llm-gateway", "backend")
    $deadline = [DateTimeOffset]::UtcNow.AddSeconds(90)
    do {
        $rows = docker compose ps --format json | ForEach-Object { $_ | ConvertFrom-Json }
        $unhealthy = @($required | Where-Object {
            $service = $_
            $row = $rows | Where-Object Service -eq $service | Select-Object -First 1
            -not $row -or $row.State -ne "running" -or $row.Health -ne "healthy"
        })
        if ($unhealthy.Count -eq 0) {
            break
        }
        Start-Sleep -Seconds 2
    } while ([DateTimeOffset]::UtcNow -lt $deadline)
    if ($unhealthy.Count -gt 0) {
        throw "services are not healthy: $($unhealthy -join ', ')"
    }

    foreach ($service in @("backend", "document-parser", "reranker-service")) {
        $containerID = docker compose ps -q $service
        if (-not $containerID) {
            continue
        }
        $runtimeUID = (docker compose exec -T $service sh -c "awk '/^Uid:/{print `$2}' /proc/1/status").Trim()
        if (-not $runtimeUID -or $runtimeUID -eq "0") {
            throw "$service is running as root"
        }
    }

    foreach ($service in @("db", "llm-gateway", "document-parser", "reranker-service")) {
        $row = $rows | Where-Object Service -eq $service | Select-Object -First 1
        if ($row.Publishers | Where-Object { $_.PublishedPort -gt 0 }) {
            throw "$service exposes a host port"
        }
    }

    $metrics = Invoke-WebRequest "http://127.0.0.1:8000/metrics"
    foreach ($name in @(
        "cortex_knowledge_index_queue",
        "cortex_research_jobs_created_total",
        "cortex_research_collector_available",
        "cortex_research_ocr_available"
    )) {
        if ($metrics.Content -notmatch "(?m)^$([regex]::Escape($name)) ") {
            throw "missing metric $name"
        }
    }

    $logs = docker compose logs --no-color --tail $LogTail backend llm-gateway
    $forbidden = @(
        "(?i)authorization:\s*(token|bearer)\s+\S+",
        "(?i)(api[_-]?key|cookie|set-cookie)\s*[=:]\s*\S+",
        "(?i)session_cookie_ciphertext",
        "(?i)BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY"
    )
    foreach ($pattern in $forbidden) {
        if (($logs -join "`n") -match $pattern) {
            throw "sensitive log pattern detected: $pattern"
        }
    }

    [pscustomobject]@{
        Status = "passed"
        HealthyServices = $required.Count
        LogLinesScanned = $logs.Count
    } | ConvertTo-Json -Compress
}
finally {
    Pop-Location
}

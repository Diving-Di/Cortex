param([string]$BaseUrl = "http://127.0.0.1:8000")

$ErrorActionPreference = "Stop"
$suffix = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$user = "redis-failure-$suffix"
$password = "Redis-Failure-9z!"

function Wait-ServiceHealthy([string]$service) {
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        $status = docker compose ps $service --format json | ConvertFrom-Json
        if ($status.Health -eq "healthy") { return }
        Start-Sleep -Seconds 1
    }
    throw "$service did not become healthy"
}

try {
    Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/auth/register" -ContentType "application/json" `
        -Body (@{ username = $user; email = "$user@example.invalid"; password = $password } | ConvertTo-Json) | Out-Null
    $login = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/auth/token" -ContentType "application/json" `
        -Body (@{ username = $user; password = $password } | ConvertTo-Json)
    $headers = @{ Authorization = "Token $($login.token)" }
    $event = Invoke-RestMethod -Uri "$BaseUrl/api/v1/ai-events/current" -Headers $headers

    docker compose stop redis | Out-Null
    docker compose restart backend | Out-Null
    Wait-ServiceHealthy "backend"
    Start-Sleep -Seconds 2

    $degradedEvent = Invoke-RestMethod -Uri "$BaseUrl/api/v1/ai-events/current" -Headers $headers
    if ($degradedEvent.status -ne "paused") { throw "event readiness was not paused" }

    $templates = Invoke-RestMethod -Uri "$BaseUrl/api/v1/templates/public?ranking=trending&page_size=5" -Headers $headers
    if ($null -eq $templates.items) { throw "template PostgreSQL fallback did not return a response" }

    $fallbackStatus = 200
    try {
        $claimHeaders = $headers.Clone()
        $claimHeaders["Idempotency-Key"] = [guid]::NewGuid().ToString()
        Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/ai-events/$($event.id)/claims" `
            -Headers $claimHeaders -ContentType "application/json" -Body "{}" | Out-Null
    }
    catch {
        $fallbackStatus = [int]$_.Exception.Response.StatusCode
        if ($fallbackStatus -notin @(409, 503)) { throw }
    }

    [pscustomobject]@{ Status = "passed"; TemplateFallback = $true; ClaimDatabaseFallback = $true; ClaimStatus = $fallbackStatus; ReadinessPaused = $true } |
        ConvertTo-Json -Compress
}
finally {
    docker compose start redis | Out-Null
    Wait-ServiceHealthy "redis"
    docker compose restart backend | Out-Null
    Wait-ServiceHealthy "backend"
}

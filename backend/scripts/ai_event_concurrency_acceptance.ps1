param(
    [string]$BaseUrl = "http://127.0.0.1:8000",
    [ValidateRange(2, 100000)][int]$Participants = 1000,
    [ValidateRange(1, 100000)][int]$Slots = 100,
    [ValidateRange(1, 1000)][int]$PrepareConcurrency = 32,
    [ValidateRange(1, 5000)][int]$ClaimConcurrency = 1000,
    [string]$TokenOutputFile = ""
)

$ErrorActionPreference = "Stop"
if ($Slots -gt $Participants) { throw "Slots must not exceed Participants" }
$suffix = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$password = "Flash-Acceptance-9z!"

function Wait-ServiceHealthy([string]$service) {
    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        $status = docker compose ps $service --format json | ConvertFrom-Json
        if ($status.Health -eq "healthy") { return }
        Start-Sleep -Seconds 1
    }
    throw "$service did not become healthy"
}

$participantRecords = 0..($Participants - 1) | ForEach-Object -Parallel {
    $username = "flash-$using:suffix-$_"
    $addressOffset = [math]::Floor($_ / 254)
    $clientIP = "198.$(18 + [math]::Floor($addressOffset / 256)).$($addressOffset % 256).$(($_ % 254) + 1)"
    $headers = @{ "X-Forwarded-For" = $clientIP }
    Invoke-RestMethod -Method Post -Uri "$using:BaseUrl/api/v1/auth/register" -Headers $headers -ContentType "application/json" `
        -Body (@{ username = $username; email = "$username@example.invalid"; password = $using:password } | ConvertTo-Json) | Out-Null
    $token = (Invoke-RestMethod -Method Post -Uri "$using:BaseUrl/api/v1/auth/token" -Headers $headers -ContentType "application/json" `
        -Body (@{ username = $username; password = $using:password } | ConvertTo-Json)).token
    $headers["Authorization"] = "Token $token"
    $event = Invoke-RestMethod -Uri "$using:BaseUrl/api/v1/ai-events/current" -Headers $headers
    $eventDate = ([datetime]$event.event_date).Date
    for ($day = 0; $day -lt $event.required_streak_days; $day++) {
        $noteDate = $eventDate.AddDays(-$day).ToString("yyyy-MM-dd")
        Invoke-RestMethod -Method Post -Uri "$using:BaseUrl/api/v1/notes" -Headers $headers -ContentType "application/json" `
            -Body (@{
                type = "normal"
                title = "Flash qualification $day"
                content = ("qualified writing content " + ("x" * 80))
                note_date = $noteDate
            } | ConvertTo-Json) | Out-Null
    }
    [pscustomobject]@{ Username = $username; Token = $token; EventID = $event.id; ClientIP = $clientIP }
} -ThrottleLimit $PrepareConcurrency

if (@($participantRecords | Select-Object -ExpandProperty EventID -Unique).Count -ne 1) {
    throw "participants resolved different events"
}
$eventState = Invoke-RestMethod -Uri "$BaseUrl/api/v1/ai-events/current" `
    -Headers @{ Authorization = "Token $($participantRecords[0].Token)"; "X-Forwarded-For" = $participantRecords[0].ClientIP }
if ($eventState.remaining_slots -ne $eventState.total_slots) {
    throw "acceptance event is not empty"
}
if ($TokenOutputFile) {
    $tokenOutput = [IO.Path]::GetFullPath($TokenOutputFile)
    $tokenOutputDirectory = [IO.Path]::GetDirectoryName($tokenOutput)
    if (-not [IO.Directory]::Exists($tokenOutputDirectory)) {
        [IO.Directory]::CreateDirectory($tokenOutputDirectory) | Out-Null
    }
    $participantRecords | ConvertTo-Json -Depth 3 | Set-Content -LiteralPath $tokenOutput -Encoding utf8NoBOM
}

$eventID = $participantRecords[0].EventID
docker compose exec -T db psql -U cortex_migrator -d cortex -v ON_ERROR_STOP=1 `
    -c "UPDATE ai_flash_events SET total_slots=$Slots,opens_at=now()+interval '5 seconds',closes_at=now()+interval '10 minutes',status='scheduled' WHERE public_id='$eventID'::uuid AND claimed_slots=0" | Out-Null
# Acceptance runs are repeatable: discard only this event's rebuildable Redis projection.
$redisPassword = if ($env:REDIS_PASSWORD) { $env:REDIS_PASSWORD } else { "change-me" }
docker compose exec -T -e REDISCLI_AUTH=$redisPassword redis sh -c `
    "redis-cli --scan --pattern 'cortex:ai-event:{$eventID}*' | xargs -r redis-cli del" | Out-Null
docker compose restart backend | Out-Null
Wait-ServiceHealthy "backend"
Start-Sleep -Seconds 6

docker compose stop llm-gateway | Out-Null
$claimStarted = [DateTimeOffset]::UtcNow
try {
    $results = $participantRecords | ForEach-Object -Parallel {
        $headers = @{
            Authorization = "Token $($_.Token)"
            "Idempotency-Key" = [guid]::NewGuid().ToString()
            "X-Forwarded-For" = $_.ClientIP
        }
        try {
            $response = Invoke-WebRequest -Method Post -Uri "$using:BaseUrl/api/v1/ai-events/$($_.EventID)/claims" `
                -Headers $headers -ContentType "application/json" -Body "{}"
            [pscustomobject]@{ Status = [int]$response.StatusCode; Code = "OK" }
        }
        catch {
            $status = [int]$_.Exception.Response.StatusCode
            $code = ""
            try { $code = ($_.ErrorDetails.Message | ConvertFrom-Json).code } catch {}
            [pscustomobject]@{ Status = $status; Code = $code }
        }
    } -ThrottleLimit $ClaimConcurrency
}
finally {
    docker compose start llm-gateway | Out-Null
    Wait-ServiceHealthy "llm-gateway"
}

$accepted = @($results | Where-Object Status -eq 200).Count
$rejected = @($results | Where-Object { $_.Status -eq 409 -and $_.Code -eq "AI_EVENT_SOLD_OUT" }).Count
if ($accepted -ne $Slots -or $rejected -ne ($Participants - $Slots)) {
    throw "unexpected claim results: accepted=$accepted sold_out=$rejected results=$($results | ConvertTo-Json -Compress)"
}
$claimDurationMs = ([DateTimeOffset]::UtcNow - $claimStarted).TotalMilliseconds

$facts = docker compose exec -T db psql -U cortex_migrator -d cortex -At -F ',' -c `
    "SELECT e.claimed_slots,count(DISTINCT c.tenant_id),count(DISTINCT j.id),(SELECT count(*) FROM ai_point_ledger l WHERE l.entry_type='grant' AND l.reference_type='ai_flash_event_reward' AND l.reference_id=e.public_id::text) FROM ai_flash_events e LEFT JOIN ai_flash_claims c ON c.event_id=e.id LEFT JOIN ai_event_jobs j ON j.claim_id=c.id WHERE e.public_id='$eventID'::uuid GROUP BY e.id,e.public_id,e.claimed_slots"
$parts = $facts.Trim().Split(',')
if ($parts.Count -ne 4 -or [int]$parts[0] -ne $Slots -or [int]$parts[1] -ne $Slots -or [int]$parts[2] -ne 0 -or [int]$parts[3] -ne $Slots) {
    throw "database facts are inconsistent: $facts"
}

[pscustomobject]@{
    Status = "passed"
    ConcurrentRequests = $Participants
    Accepted = $accepted
    SoldOut = $rejected
    Claims = [int]$parts[1]
    Jobs = [int]$parts[2]
    RewardGrants = [int]$parts[3]
    ClaimDurationMs = [math]::Round($claimDurationMs, 2)
} | ConvertTo-Json -Compress

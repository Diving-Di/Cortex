param([string]$BaseUrl = "http://127.0.0.1:8000")

$ErrorActionPreference = "Stop"
$suffix = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$password = "Flash-Acceptance-9z!"
$participants = @()

function Wait-ServiceHealthy([string]$service) {
    for ($attempt = 0; $attempt -lt 60; $attempt++) {
        $status = docker compose ps $service --format json | ConvertFrom-Json
        if ($status.Health -eq "healthy") { return }
        Start-Sleep -Seconds 1
    }
    throw "$service did not become healthy"
}

for ($index = 0; $index -lt 12; $index++) {
    $username = "flash-$suffix-$index"
    Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/auth/register" -ContentType "application/json" `
        -Body (@{ username = $username; email = "$username@example.invalid"; password = $password } | ConvertTo-Json) | Out-Null
    $token = (Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/auth/token" -ContentType "application/json" `
        -Body (@{ username = $username; password = $password } | ConvertTo-Json)).token
    $headers = @{ Authorization = "Token $token" }
    $event = Invoke-RestMethod -Uri "$BaseUrl/api/v1/ai-events/current" -Headers $headers
    if ($event.remaining_slots -ne $event.total_slots) { throw "acceptance event is not empty" }
    $eventDate = ([datetime]$event.event_date).Date
    for ($day = 0; $day -lt $event.required_streak_days; $day++) {
        $noteDate = $eventDate.AddDays(-$day).ToString("yyyy-MM-dd")
        Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/notes" -Headers $headers -ContentType "application/json" `
            -Body (@{
                type = "normal"
                title = "Flash qualification $day"
                content = ("qualified writing content " + ("x" * 80))
                note_date = $noteDate
            } | ConvertTo-Json) | Out-Null
    }
    $participants += [pscustomobject]@{ Token = $token; EventID = $event.id }
}

$eventID = $participants[0].EventID
docker compose exec -T db psql -U diary_migrator -d diary_listener -v ON_ERROR_STOP=1 `
    -c "UPDATE ai_flash_events SET opens_at=now()-interval '1 second',closes_at=now()+interval '10 minutes',status='scheduled' WHERE public_id='$eventID'::uuid AND claimed_slots=0" | Out-Null
docker compose restart backend | Out-Null
Wait-ServiceHealthy "backend"
Start-Sleep -Seconds 3

docker compose stop llm-gateway | Out-Null
try {
    $results = $participants | ForEach-Object -Parallel {
        $headers = @{
            Authorization = "Token $($_.Token)"
            "Idempotency-Key" = [guid]::NewGuid().ToString()
        }
        try {
            $response = Invoke-WebRequest -Method Post -Uri "$using:BaseUrl/api/v1/ai-events/$($_.EventID)/claims" `
                -Headers $headers -ContentType "application/json" -Body "{}"
            [pscustomobject]@{ Status = [int]$response.StatusCode }
        }
        catch {
            [pscustomobject]@{ Status = [int]$_.Exception.Response.StatusCode }
        }
    } -ThrottleLimit 12
}
finally {
    docker compose start llm-gateway | Out-Null
    Wait-ServiceHealthy "llm-gateway"
}

$accepted = @($results | Where-Object Status -eq 200).Count
$rejected = @($results | Where-Object Status -eq 409).Count
if ($accepted -ne 10 -or $rejected -ne 2) {
    throw "unexpected claim results: accepted=$accepted rejected=$rejected statuses=$($results.Status -join ',')"
}

$facts = docker compose exec -T db psql -U diary_migrator -d diary_listener -At -F ',' -c `
    "SELECT e.claimed_slots,count(DISTINCT c.tenant_id),count(DISTINCT j.id),(SELECT count(*) FROM ai_point_ledger l WHERE l.entry_type='grant' AND l.reference_type='ai_flash_event_reward' AND l.reference_id=e.public_id::text) FROM ai_flash_events e LEFT JOIN ai_flash_claims c ON c.event_id=e.id LEFT JOIN ai_event_jobs j ON j.claim_id=c.id WHERE e.public_id='$eventID'::uuid GROUP BY e.id,e.public_id,e.claimed_slots"
$parts = $facts.Trim().Split(',')
if ($parts.Count -ne 4 -or [int]$parts[0] -ne 10 -or [int]$parts[1] -ne 10 -or [int]$parts[2] -ne 0 -or [int]$parts[3] -ne 10) {
    throw "database facts are inconsistent: $facts"
}

[pscustomobject]@{
    Status = "passed"
    ConcurrentRequests = 12
    Accepted = $accepted
    SoldOut = $rejected
    Claims = [int]$parts[1]
    Jobs = [int]$parts[2]
    RewardGrants = [int]$parts[3]
} | ConvertTo-Json -Compress

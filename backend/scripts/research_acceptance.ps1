param(
    [string]$BaseUrl = "http://127.0.0.1:8000",
    [string]$Username = "research_acceptance_$([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())",
    [string]$Password = "Acceptance-Research-Only-42!"
)

$ErrorActionPreference = "Stop"

function Invoke-Json {
    param([string]$Method, [string]$Uri, [hashtable]$Headers, [object]$Body)
    $arguments = @{
        Method = $Method
        Uri = $Uri
        Headers = $Headers
        ContentType = "application/json"
    }
    if ($null -ne $Body) {
        $arguments.Body = $Body | ConvertTo-Json -Depth 8
    }
    Invoke-RestMethod @arguments
}

$registration = Invoke-Json -Method Post -Uri "$BaseUrl/api/v1/auth/register" -Headers @{} -Body @{
    username = $Username
    email = "$Username@example.test"
    password = $Password
}
$token = $registration.token
if (-not $token) {
    $login = Invoke-Json -Method Post -Uri "$BaseUrl/api/v1/auth/token" -Headers @{} -Body @{
        username = $Username
        password = $Password
    }
    $token = $login.token
}
$headers = @{ Authorization = "Token $token" }

$idempotencyKey = "acceptance-$([guid]::NewGuid())"
$jobBody = @{
    mode = "urls"
    urls = @("https://www.xiaohongshu.com/explore/000000000000000000000000")
    target_count = 1
    idempotency_key = $idempotencyKey
}
$job = Invoke-Json -Method Post -Uri "$BaseUrl/api/v1/research/jobs" -Headers $headers -Body $jobBody
if ($job.status -ne "queued") {
    throw "Research job was not queued"
}
$duplicate = Invoke-Json -Method Post -Uri "$BaseUrl/api/v1/research/jobs" -Headers $headers -Body $jobBody
if ($duplicate.id -ne $job.id) {
    throw "Research idempotency key created duplicate jobs"
}

$jobs = Invoke-Json -Method Get -Uri "$BaseUrl/api/v1/research/jobs" -Headers $headers -Body $null
if (-not ($jobs.items | Where-Object { $_.id -eq $job.id })) {
    throw "Queued research job was not returned"
}

Invoke-Json -Method Post -Uri "$BaseUrl/api/v1/research/jobs/$($job.id)/cancel" -Headers $headers -Body $null | Out-Null
$updated = Invoke-Json -Method Get -Uri "$BaseUrl/api/v1/research/jobs/$($job.id)" -Headers $headers -Body $null
if ($updated.status -notin @("cancelled", "collecting")) {
    throw "Research cancellation was not persisted"
}

Write-Host "Research acceptance passed for job $($job.id)"

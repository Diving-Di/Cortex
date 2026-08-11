param(
    [string]$BaseUrl = "http://127.0.0.1:8000",
    [Parameter(Mandatory = $true)]
    [string]$Username,
    [string]$Password,
    [Parameter(Mandatory = $true)]
    [ValidatePattern("^https://www\.xiaohongshu\.com/(explore|discovery/item)/")]
    [string]$SourceUrl,
    [int]$TimeoutSeconds = 240
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($Password)) {
    $securePassword = Read-Host "Diary Listener password" -AsSecureString
    $credential = [pscredential]::new($Username, $securePassword)
    $Password = $credential.GetNetworkCredential().Password
}

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

$login = Invoke-Json -Method Post -Uri "$BaseUrl/api/v1/auth/token" -Headers @{} -Body @{
    username = $Username
    password = $Password
}
if (-not $login.token) {
    throw "Login did not return a token"
}
$headers = @{ Authorization = "Token $($login.token)" }

try {
    $authorization = Invoke-Json -Method Get -Uri "$BaseUrl/api/v1/research/xhs/authorization" -Headers $headers -Body $null
} catch {
    throw "XHS authorization is missing or unavailable. Open /research, finish QR authorization, then rerun this script."
}
if ($authorization.status -ne "authorized") {
    throw "XHS authorization is '$($authorization.status)', expected 'authorized'"
}

$job = Invoke-Json -Method Post -Uri "$BaseUrl/api/v1/research/jobs" -Headers $headers -Body @{
    mode = "urls"
    urls = @($SourceUrl)
    target_count = 1
    idempotency_key = "live-acceptance-$([guid]::NewGuid())"
}
if ($job.status -ne "queued") {
    throw "Research job was not queued"
}

$deadline = [DateTimeOffset]::UtcNow.AddSeconds($TimeoutSeconds)
do {
    Start-Sleep -Seconds 3
    $job = Invoke-Json -Method Get -Uri "$BaseUrl/api/v1/research/jobs/$($job.id)" -Headers $headers -Body $null
} while (
    $job.status -notin @("completed", "failed", "cancelled") -and
    [DateTimeOffset]::UtcNow -lt $deadline
)

if ($job.status -ne "completed") {
    throw "Research job $($job.id) ended as '$($job.status)' with code '$($job.last_error_code)'"
}

$sourceList = Invoke-Json -Method Get `
    -Uri "$BaseUrl/api/v1/research/sources?job_id=$($job.id)&limit=20&offset=0" `
    -Headers $headers `
    -Body $null
$source = $sourceList.items | Where-Object { $_.job_id -eq $job.id } | Select-Object -First 1
if ($null -eq $source) {
    throw "Completed job $($job.id) did not create a source"
}

$source = Invoke-Json -Method Get -Uri "$BaseUrl/api/v1/research/sources/$($source.id)" -Headers $headers -Body $null
if ($source.status -ne "pending_review") {
    throw "Source $($source.id) is '$($source.status)' instead of 'pending_review'"
}
if ([string]::IsNullOrWhiteSpace($source.title)) {
    throw "Source title is empty"
}
if ([string]::IsNullOrWhiteSpace($source.formatted_content) -and [string]::IsNullOrWhiteSpace($source.raw_content)) {
    throw "Source content is empty"
}
if ($source.content_completeness -le 0) {
    throw "Source content completeness was not calculated"
}
if ($null -eq $source.draft -or [string]::IsNullOrWhiteSpace($source.draft.summary)) {
    throw "Research draft or summary is missing"
}

[pscustomobject]@{
    Result = "passed"
    JobId = $job.id
    SourceId = $source.id
    SourceStatus = $source.status
    Title = $source.title
    ParseStrategy = $source.parse_strategy
    ContentCompleteness = $source.content_completeness
    FormatStatus = $source.format_status
    AssetCount = @($source.assets).Count
} | Format-List

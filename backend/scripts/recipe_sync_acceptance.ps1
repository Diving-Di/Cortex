param(
    [string]$BaseUrl = "http://127.0.0.1:8000",
    [int]$ReadyTimeoutSeconds = 900
)

$ErrorActionPreference = "Stop"
$suffix = [Guid]::NewGuid().ToString("N")
$username = "recipe_$suffix"
$password = "Recipe-Acceptance-$suffix!"

function Invoke-Api {
    param([string]$Method, [string]$Path, $Body, [string]$Token)
    $headers = @{}
    if ($Token) { $headers.Authorization = "Token $Token" }
    $parameters = @{
        Uri = "$BaseUrl$Path"
        Method = $Method
        Headers = $headers
        ContentType = "application/json"
    }
    if ($null -ne $Body) { $parameters.Body = ($Body | ConvertTo-Json -Depth 10) }
    Invoke-RestMethod @parameters
}

$deadline = (Get-Date).AddSeconds($ReadyTimeoutSeconds)
do {
    try {
        $ready = Invoke-RestMethod -Uri "$BaseUrl/readyz"
        if ($ready.status -eq "ready") { break }
    } catch {}
    Start-Sleep -Seconds 5
} while ((Get-Date) -lt $deadline)
if (-not $ready -or $ready.status -ne "ready") {
    throw "readyz did not become ready before timeout"
}

$registration = Invoke-Api POST "/api/v1/auth/register" @{
    username = $username
    email = "$username@example.invalid"
    password = $password
} ""
$token = if ($registration.token) { $registration.token } else {
    (Invoke-Api POST "/api/v1/auth/login" @{ username = $username; password = $password } "").token
}
if (-not $token) { throw "registration/login did not return a token" }

$first = Invoke-Api GET "/api/v1/recipes/today" $null $token
$second = Invoke-Api GET "/api/v1/recipes/today" $null $token
if ($first.recipe.id -ne $second.recipe.id) { throw "daily recommendation is not deterministic" }
if ($first.suggested_questions.Count -ne 3) { throw "expected exactly three suggested questions" }

$preferences = Invoke-Api GET "/api/v1/settings/preferences" $null $token
$updated = Invoke-Api PUT "/api/v1/settings/preferences" @{
    dietary_restrictions = @("__acceptance_impossible_term__")
    timezone = "Asia/Shanghai"
    version = $preferences.version
} $token
if ($updated.version -le $preferences.version) { throw "preference version did not advance" }

try {
    Invoke-Api PUT "/api/v1/settings/preferences" @{
        dietary_restrictions = @("stale")
        timezone = "Asia/Shanghai"
        version = $preferences.version
    } $token | Out-Null
    throw "stale preference update unexpectedly succeeded"
} catch {
    if ($_.Exception.Response.StatusCode.value__ -ne 409) { throw }
}

$today = Invoke-Api GET "/api/v1/recipes/today" $null $token
if (-not $today.corpus_revision -or -not $today.recipe.title) {
    throw "today response is missing corpus revision or recipe"
}

Write-Host "Recipe acceptance passed: revision=$($today.corpus_revision) recipe=$($today.recipe.id)"

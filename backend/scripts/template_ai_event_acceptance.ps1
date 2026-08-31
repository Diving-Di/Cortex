param(
    [string]$BaseUrl = "http://127.0.0.1:8000"
)
$ErrorActionPreference = "Stop"
$suffix = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$user = "market$suffix"
$password = "acceptance-$suffix"
$email = "$user@example.invalid"
$clientHeaders = @{ "X-Forwarded-For" = "192.0.2.$(Get-Random -Minimum 1 -Maximum 255)" }
Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/auth/register" -Headers $clientHeaders -ContentType "application/json" -Body (@{username=$user;email=$email;password=$password}|ConvertTo-Json) | Out-Null
$login = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/auth/token" -Headers $clientHeaders -ContentType "application/json" -Body (@{username=$user;password=$password}|ConvertTo-Json)
$headers = $clientHeaders.Clone()
$headers["Authorization"] = "Token $($login.token)"
Invoke-RestMethod -Method Put -Uri "$BaseUrl/api/v1/public-profile" -Headers $headers -ContentType "application/json" -Body (@{nickname="验收用户";discoverable=$true}|ConvertTo-Json) | Out-Null
$template = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/templates" -Headers $headers -ContentType "application/json" -Body (@{title="验收模板";description="模板广场验收";content_markdown="# 今日复盘`n`n- 完成事项";category="reflection"}|ConvertTo-Json)
$privateUseHeaders = $headers.Clone(); $privateUseHeaders["Idempotency-Key"]=[guid]::NewGuid().ToString()
$privateUsed1 = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/templates/$($template.id)/use" -Headers $privateUseHeaders -ContentType "application/json" -Body "{}"
$privateUsed2 = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/templates/$($template.id)/use" -Headers $privateUseHeaders -ContentType "application/json" -Body "{}"
if ($privateUsed1.note_id -ne $privateUsed2.note_id) { throw "private template use is not idempotent" }
$published = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/templates/$($template.id)/publish" -Headers $headers -ContentType "application/json" -Body "{}"
$recommended = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/templates/public?ranking=recommended&page_size=100" -Headers $headers
if (@($recommended.items | Where-Object { $_.public_id -eq $published.public_id }).Count -ne 1) { throw "recommended ranking did not return an unused template" }
$useHeaders = $headers.Clone(); $useHeaders["Idempotency-Key"]=[guid]::NewGuid().ToString()
$used1 = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/templates/public/$($published.public_id)/use" -Headers $useHeaders -ContentType "application/json" -Body "{}"
$used2 = Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/templates/public/$($published.public_id)/use" -Headers $useHeaders -ContentType "application/json" -Body "{}"
if (!$used1.note_id -or $used1.note_id -ne $used2.note_id) { throw "public template use is not idempotent" }
Invoke-RestMethod -Method Put -Uri "$BaseUrl/api/v1/templates/public/$($published.public_id)/like" -Headers $headers -ContentType "application/json" -Body "{}" | Out-Null
Invoke-RestMethod -Method Put -Uri "$BaseUrl/api/v1/templates/public/$($published.public_id)/favorite" -Headers $headers -ContentType "application/json" -Body "{}" | Out-Null
Invoke-RestMethod -Method Post -Uri "$BaseUrl/api/v1/templates/public/$($published.public_id)/views" -Headers $headers -ContentType "application/json" -Body "{}" | Out-Null
foreach ($ranking in @("daily","trending","new")) {
    $found = $false
    for ($attempt = 0; $attempt -lt 10 -and -not $found; $attempt++) {
        $list = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/templates/public?ranking=$ranking&page_size=100" -Headers $headers
        $found = @($list.items | Where-Object { $_.public_id -eq $published.public_id }).Count -eq 1
        if (-not $found) { Start-Sleep -Seconds 1 }
    }
    if (-not $found) { throw "ranking $ranking did not return the published template" }
}
$recommendedAfterUse = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/templates/public?ranking=recommended&page_size=100" -Headers $headers
if (@($recommendedAfterUse.items | Where-Object { $_.public_id -eq $published.public_id }).Count -ne 0) { throw "recently used template remained in recommendations" }
$detail = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/templates/public/$($published.public_id)" -Headers $headers
if (!$detail.liked -or !$detail.favorited) { throw "template reactions were not persisted" }
$event = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/ai-events/current" -Headers $headers
if ($event.total_slots -lt 1 -or $event.points_reward -lt 1 -or $event.remaining_slots -lt 0 -or $event.remaining_slots -gt $event.total_slots) {
    throw "event configuration mismatch"
}
$balance = Invoke-RestMethod -Method Get -Uri "$BaseUrl/api/v1/ai-points/balance" -Headers $headers
if ($balance.available -lt 100) { throw "point account was not initialized" }
Write-Output "template and AI event acceptance passed"

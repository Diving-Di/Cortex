$ErrorActionPreference = "Stop"

$base = if ($env:GO_BACKEND_URL) { $env:GO_BACKEND_URL } else { "http://127.0.0.1:8000" }
$suffix = [guid]::NewGuid().ToString("N").Substring(0, 8)
$users = @("nonai_${suffix}_a", "nonai_${suffix}_b")
$clientHeaders = @{ "X-Forwarded-For" = "198.51.100.$(Get-Random -Minimum 1 -Maximum 255)" }
$headers = @()
$exportFile = Join-Path ([IO.Path]::GetTempPath()) "cortex-non-ai-export-$suffix.zip"

try {
    foreach ($username in $users) {
        $registration = @{
            username = $username
            email = "$username@example.invalid"
            password = "correct-horse-battery"
        } | ConvertTo-Json
        Invoke-RestMethod "$base/api/v1/auth/register" `
            -Method Post -Headers $clientHeaders -ContentType "application/json" -Body $registration | Out-Null
        $login = Invoke-RestMethod "$base/api/v1/auth/token" `
            -Method Post -Headers $clientHeaders -ContentType "application/json" `
            -Body (@{ username = $username; password = "correct-horse-battery" } | ConvertTo-Json)
        $authenticatedHeaders = $clientHeaders.Clone()
        $authenticatedHeaders["Authorization"] = "Token $($login.token)"
        $headers += $authenticatedHeaders
    }

    $renamed = Invoke-RestMethod "$base/api/v1/tenant" `
        -Method Patch -Headers $headers[0] -ContentType "application/json" `
        -Body (@{ name = "Go 个人空间" } | ConvertTo-Json)
    $note = Invoke-RestMethod "$base/api/v1/notes" `
        -Method Post -Headers $headers[0] -ContentType "application/json" `
        -Body (@{
            type = "normal"
            title = "中文旅行记录"
            content = "今天去了杭州西湖，天气很好。"
        } | ConvertTo-Json)
    $tag = Invoke-RestMethod "$base/api/v1/tags" `
        -Method Post -Headers $headers[0] -ContentType "application/json" `
        -Body (@{ name = "旅行"; color = "#1677ff" } | ConvertTo-Json)
    $assigned = Invoke-RestMethod "$base/api/v1/notes/$($note.id)/tags" `
        -Method Put -Headers $headers[0] -ContentType "application/json" `
        -Body (@{ tag_ids = @($tag.id) } | ConvertTo-Json)
    $search = Invoke-RestMethod "$base/api/v1/search?q=西湖&tag_id=$($tag.id)" `
        -Headers $headers[0]
    $isolatedSearch = Invoke-RestMethod "$base/api/v1/search?q=西湖" -Headers $headers[1]
    $dashboard = Invoke-RestMethod "$base/api/v1/dashboard?timezone=Asia/Shanghai" `
        -Headers $headers[0]

    $readme = Get-Item (Join-Path $PSScriptRoot "..\README.md")
    $upload = Invoke-RestMethod "$base/api/v1/attachments?note_id=$($note.id)" `
        -Method Post -Headers $headers[0] -Form @{ file = $readme }
    $crossAttachment = 0
    try {
        Invoke-WebRequest "$base/api/v1/attachments/$($upload.id)" `
            -Headers $headers[1] -ErrorAction Stop | Out-Null
    } catch {
        $crossAttachment = [int]$_.Exception.Response.StatusCode
    }

    Invoke-WebRequest "$base/api/v1/exports/markdown" `
        -Method Post -Headers $headers[0] -OutFile $exportFile
    Invoke-WebRequest "$base/api/v1/tenant" -Method Delete -Headers $headers[0] | Out-Null
    $deletedStatus = 0
    try {
        Invoke-WebRequest "$base/api/v1/notes" -Headers $headers[0] -ErrorAction Stop | Out-Null
    } catch {
        $deletedStatus = [int]$_.Exception.Response.StatusCode
    }
    [pscustomobject]@{
        Suffix = $suffix
        TenantRenamed = $renamed.name
        AssignedTags = @($assigned).Count
        SearchTotal = $search.total
        OtherTenantSearch = $isolatedSearch.total
        DashboardNotes = $dashboard.statistics.notes
        AttachmentSize = $upload.size
        CrossTenantAttachment = $crossAttachment
        ExportBytes = (Get-Item $exportFile).Length
        DeletedTenantStatus = $deletedStatus
    } | ConvertTo-Json -Compress
} finally {
    Remove-Item -LiteralPath $exportFile -Force -ErrorAction SilentlyContinue
}

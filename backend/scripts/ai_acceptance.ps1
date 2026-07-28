param(
    [string]$BaseURL = "http://127.0.0.1:8000"
)

$ErrorActionPreference = "Stop"

function Invoke-AIStream {
    param(
        [string]$Path,
        [hashtable]$Body,
        [string]$Token
    )

    $handler = [System.Net.Http.HttpClientHandler]::new()
    $client = [System.Net.Http.HttpClient]::new($handler)
    $client.Timeout = [TimeSpan]::FromMinutes(10)
    try {
        $request = [System.Net.Http.HttpRequestMessage]::new(
            [System.Net.Http.HttpMethod]::Post,
            "$BaseURL$Path"
        )
        $request.Headers.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new(
            "Token",
            $Token
        )
        $json = $Body | ConvertTo-Json -Depth 8 -Compress
        $request.Content = [System.Net.Http.StringContent]::new(
            $json,
            [System.Text.Encoding]::UTF8,
            "application/json"
        )
        $response = $client.Send(
            $request,
            [System.Net.Http.HttpCompletionOption]::ResponseHeadersRead
        )
        if (-not $response.IsSuccessStatusCode) {
            $errorBody = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
            throw "AI stream $Path failed with HTTP $([int]$response.StatusCode): $errorBody"
        }
        $reader = [System.IO.StreamReader]::new(
            $response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
        )
        $output = [System.Text.StringBuilder]::new()
        while (-not $reader.EndOfStream) {
            $line = $reader.ReadLine()
            if (-not $line.StartsWith("data: ")) {
                continue
            }
            $data = $line.Substring(6)
            if ($data -eq "[DONE]") {
                break
            }
            $event = $data | ConvertFrom-Json
            if ($event.content) {
                [void]$output.Append([string]$event.content)
            }
        }
        return $output.ToString()
    } finally {
        $client.Dispose()
        $handler.Dispose()
    }
}

$suffix = [guid]::NewGuid().ToString("N").Substring(0, 8)
$username = "realai_$suffix"
$password = "correct-horse-battery"
$registration = @{
    username = $username
    email = "$username@example.invalid"
    password = $password
} | ConvertTo-Json
Invoke-RestMethod "$BaseURL/api/v1/auth/register" `
    -Method Post -ContentType "application/json" -Body $registration | Out-Null
$login = Invoke-RestMethod "$BaseURL/api/v1/auth/login" `
    -Method Post -ContentType "application/json" `
    -Body (@{ username = $username; password = $password } | ConvertTo-Json)
$headers = @{ Authorization = "Token $($login.token)" }

$settings = Invoke-RestMethod "$BaseURL/api/v1/settings/ai" -Headers $headers
if (-not $settings.configured) {
    throw "AI is not configured in the Go container"
}

$chat = Invoke-AIStream "/api/v1/ai/stream" `
    @{ prompt = "只回复：连接成功" } $login.token
$organizedText = Invoke-AIStream "/api/v1/ai/organize" `
    @{ content = "今天完成了 Diary Listener 的 Go 迁移验收，并验证了租户隔离。"; style = "structured" } `
    $login.token
$organized = Invoke-RestMethod "$BaseURL/api/v1/ai/organize/confirm" `
    -Method Post -Headers $headers -ContentType "application/json" `
    -Body (@{
        title = "Go 迁移验收"
        content = $organizedText
        summary = "Go 迁移验收记录"
    } | ConvertTo-Json)

$today = Get-Date -Format "yyyy-MM-dd"
$source = Invoke-RestMethod "$BaseURL/api/v1/notes" `
    -Method Post -Headers $headers -ContentType "application/json" `
    -Body (@{
        type = "normal"
        title = "真实 AI 验收来源"
        content = "今天完成了 Go 后端的真实 AI SSE、报告引用和回忆问答验收。"
        note_date = $today
    } | ConvertTo-Json)
$preview = Invoke-RestMethod "$BaseURL/api/v1/reports/preview" `
    -Method Post -Headers $headers -ContentType "application/json" `
    -Body (@{ type = "daily"; anchor_date = $today } | ConvertTo-Json)
$reportText = Invoke-AIStream "/api/v1/reports/generate" `
    @{ type = "daily"; anchor_date = $today } $login.token
$sourceIDs = @($preview.sources | ForEach-Object { $_.id })
$report = Invoke-RestMethod "$BaseURL/api/v1/reports/confirm" `
    -Method Post -Headers $headers -ContentType "application/json" `
    -Body (@{
        type = "daily"
        anchor_date = $today
        title = "$today 日报"
        content = $reportText
        source_ids = $sourceIDs
        overwrite = $false
    } | ConvertTo-Json -Depth 5)
$reportSources = Invoke-RestMethod "$BaseURL/api/v1/reports/$($report.id)/sources" `
    -Headers $headers

$notebookText = Invoke-AIStream "/api/v1/knowledge/chat" `
    @{ question = "Go 后端验收完成了什么？"; source_scope = "growth" } $login.token
$conversations = Invoke-RestMethod "$BaseURL/api/v1/conversations" -Headers $headers
$notebookConversation = @($conversations.items | Sort-Object id -Descending)[0]
$conversation = Invoke-RestMethod `
    "$BaseURL/api/v1/conversations/$($notebookConversation.id)" -Headers $headers
$assistantMessage = @($conversation.messages | Where-Object { $_.role -eq "assistant" })[-1]
$notebookSources = Invoke-RestMethod `
    "$BaseURL/api/v1/knowledge/messages/$($assistantMessage.id)/sources" -Headers $headers
if (@($notebookSources).Count -lt 1 -or
    @($notebookSources | Where-Object { $_.source_type -eq "growth_note" }).Count -lt 1) {
    throw "Growth answer did not persist a unified growth_note source."
}

[pscustomobject]@{
    Configured = $settings.configured
    Model = $settings.model
    ChatCharacters = $chat.Length
    OrganizeCharacters = $organizedText.Length
    OrganizedNoteID = $organized.id
    ReportCharacters = $reportText.Length
    ReportNoteID = $report.id
    ReportSourceCount = @($reportSources).Count
    NotebookCharacters = $notebookText.Length
    NotebookSourceCount = @($notebookSources).Count
} | ConvertTo-Json -Compress

param(
    [string]$BaseURL = "http://127.0.0.1:8000"
)

$ErrorActionPreference = "Stop"
$stamp = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$password = "Backup-Acceptance-9z!"
$sourceUser = "backup-src-$stamp"
$targetUser = "backup-dst-$stamp"
$temporary = [IO.Path]::GetTempPath()
$sourceFile = Join-Path $temporary "cortex-source-$stamp.txt"
$archiveFile = Join-Path $temporary "cortex-backup-$stamp.zip"

function New-AcceptanceUser([string]$username) {
    Invoke-RestMethod -Method Post -Uri "$BaseURL/api/v1/auth/register" `
        -ContentType "application/json" `
        -Body (@{ username = $username; email = "$username@example.test"; password = $password } | ConvertTo-Json) |
        Out-Null
    return (Invoke-RestMethod -Method Post -Uri "$BaseURL/api/v1/auth/login" `
        -ContentType "application/json" `
        -Body (@{ username = $username; password = $password } | ConvertTo-Json)).token
}

try {
    $sourceToken = New-AcceptanceUser $sourceUser
    $targetToken = New-AcceptanceUser $targetUser
    $sourceHeaders = @{ Authorization = "Token $sourceToken" }
    $targetHeaders = @{ Authorization = "Token $targetToken" }

    $note = Invoke-RestMethod -Method Post -Uri "$BaseURL/api/v1/notes" `
        -Headers $sourceHeaders -ContentType "application/json" `
        -Body (@{ type = "normal"; title = "Backup acceptance"; content = "tenant scoped body" } | ConvertTo-Json)
    Set-Content -LiteralPath $sourceFile -Value "private attachment body" -NoNewline
    Invoke-RestMethod -Method Post -Uri "$BaseURL/api/v1/attachments?note_id=$($note.id)" `
        -Headers $sourceHeaders -Form @{ file = Get-Item -LiteralPath $sourceFile } | Out-Null
    Invoke-RestMethod -Method Post -Uri "$BaseURL/api/v1/research/jobs" `
        -Headers $sourceHeaders -ContentType "application/json" `
        -Body (@{
            mode = "urls"
            urls = @("https://www.xiaohongshu.com/explore/000000000000000000000000")
            target_count = 1
            idempotency_key = "backup-$stamp"
        } | ConvertTo-Json) | Out-Null

    Invoke-WebRequest -Uri "$BaseURL/api/v1/backups/full" -Headers $sourceHeaders -OutFile $archiveFile
    Invoke-RestMethod -Method Post -Uri "$BaseURL/api/v1/backups/full/restore" `
        -Headers $targetHeaders -ContentType "application/zip" -InFile $archiveFile | Out-Null

    $notes = Invoke-RestMethod -Uri "$BaseURL/api/v1/notes" -Headers $targetHeaders
    if ($notes.total -ne 1 -or $notes.items[0].title -ne "Backup acceptance") {
        throw "restored note mismatch"
    }
    if ($notes.items[0].id -eq $note.id) {
        throw "note ID was not remapped"
    }
    $attachments = Invoke-RestMethod -Uri "$BaseURL/api/v1/attachments/note/$($notes.items[0].id)" `
        -Headers $targetHeaders
    if ($attachments.Count -ne 1) {
        throw "restored attachment count mismatch"
    }
    $download = Invoke-WebRequest -Uri "$BaseURL/api/v1/attachments/$($attachments[0].id)" `
        -Headers $targetHeaders
    if ($download.Content -ne "private attachment body") {
        throw "restored attachment body mismatch"
    }
    $researchJobs = Invoke-RestMethod -Uri "$BaseURL/api/v1/research/jobs" -Headers $targetHeaders
    if ($researchJobs.total -lt 1) {
        throw "research jobs were not restored"
    }
    try {
        Invoke-RestMethod -Method Post -Uri "$BaseURL/api/v1/backups/full/restore" `
            -Headers $targetHeaders -ContentType "application/zip" -InFile $archiveFile | Out-Null
        throw "restore unexpectedly accepted a non-empty tenant"
    }
    catch {
        if ($_.Exception.Response.StatusCode -ne 409) {
            throw
        }
    }

    [pscustomobject]@{
        Status = "passed"
        ArchiveBytes = (Get-Item -LiteralPath $archiveFile).Length
        RestoredNotes = $notes.total
        RestoredAttachments = $attachments.Count
        RestoredResearchJobs = $researchJobs.total
    } | ConvertTo-Json -Compress
}
finally {
    Remove-Item -LiteralPath $sourceFile, $archiveFile -Force -ErrorAction SilentlyContinue
}

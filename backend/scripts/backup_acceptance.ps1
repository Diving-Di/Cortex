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

    Invoke-RestMethod -Method Put -Uri "$BaseURL/api/v1/public-profile" `
        -Headers $sourceHeaders -ContentType "application/json" `
        -Body (@{ nickname = "备份验收$stamp"; discoverable = $true } | ConvertTo-Json) | Out-Null
    $template = Invoke-RestMethod -Method Post -Uri "$BaseURL/api/v1/templates" `
        -Headers $sourceHeaders -ContentType "application/json" `
        -Body (@{
            title = "Backup private template"
            description = "must restore as private"
            content_markdown = "# Restored template`n`nPrivate body"
            category = "reflection"
        } | ConvertTo-Json)
    $published = Invoke-RestMethod -Method Post -Uri "$BaseURL/api/v1/templates/$($template.id)/publish" `
        -Headers $sourceHeaders -ContentType "application/json" -Body "{}"
    Invoke-RestMethod -Method Put -Uri "$BaseURL/api/v1/templates/public/$($published.public_id)/favorite" `
        -Headers $sourceHeaders -ContentType "application/json" -Body "{}" | Out-Null

    Invoke-WebRequest -Uri "$BaseURL/api/v1/backups/full" -Headers $sourceHeaders -OutFile $archiveFile

    $archive = [IO.Compression.ZipFile]::OpenRead($archiveFile)
    try {
        $manifestEntry = $archive.GetEntry("manifest.json")
        if (!$manifestEntry) { throw "backup manifest missing" }
        $reader = [IO.StreamReader]::new($manifestEntry.Open())
        try { $manifest = ($reader.ReadToEnd() | ConvertFrom-Json) } finally { $reader.Dispose() }
        if ($manifest.tables.writing_templates.Count -ne 1) { throw "private template missing from backup" }
        if ($manifest.tables.template_reactions.Count -ne 1 -or $manifest.tables.template_reactions[0].kind -ne "favorite") {
            throw "favorite reaction missing from backup"
        }
        foreach ($excluded in @("published_template_snapshots", "template_public_stats", "template_reports", "ai_flash_events", "ai_flash_claims")) {
            if ($manifest.tables.PSObject.Properties.Name -contains $excluded) { throw "excluded table $excluded leaked into backup" }
        }
    }
    finally { $archive.Dispose() }

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
    $templates = Invoke-RestMethod -Uri "$BaseURL/api/v1/templates/mine" -Headers $targetHeaders
    if ($templates.items.Count -ne 1 -or $templates.items[0].title -ne "Backup private template" -or $templates.items[0].status -ne "private") {
        throw "restored private template mismatch"
    }
    $publicDetail = Invoke-RestMethod -Uri "$BaseURL/api/v1/templates/public/$($published.public_id)" -Headers $targetHeaders
    if (!$publicDetail.favorited) { throw "restored favorite mismatch" }
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
        RestoredTemplates = $templates.items.Count
        RestoredFavorites = 1
    } | ConvertTo-Json -Compress
}
finally {
    Remove-Item -LiteralPath $sourceFile, $archiveFile -Force -ErrorAction SilentlyContinue
}

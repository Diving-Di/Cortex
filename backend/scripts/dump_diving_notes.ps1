#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Dump Diving's existing notes from the database for inspection.
  Run this first to see what notes exist before running the full eval.
#>

$ErrorActionPreference = "Stop"
$repositoryDir = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)

Write-Host "Querying Diving's notes from database..." -ForegroundColor Yellow

# Get full note content
$notes = docker compose -f "$repositoryDir/docker-compose.yml" exec -T db `
    psql -U diary_migrator -d diary_listener -t -A -F '|||' -c @"
SELECT n.id, n.title, n.content, n.created_at,
       CASE WHEN d.id IS NOT NULL THEN d.status ELSE 'not_indexed' END as kb_status,
       CASE WHEN d.id IS NOT NULL THEN d.active_index_version ELSE 0 END as kb_version,
       CASE WHEN d.id IS NOT NULL THEN d.source_type ELSE '' END as source_type
FROM notes n
JOIN users u ON u.id = n.user_id
LEFT JOIN knowledge_documents d ON d.tenant_id = n.tenant_id AND d.note_id = n.id AND d.deleted_at IS NULL
WHERE u.username = 'Diving'
ORDER BY n.created_at DESC;
"@ 2>&1

if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: $notes" -ForegroundColor Red
    exit 1
}

$count = 0
$outDir = Join-Path $repositoryDir "backend/testdata/rag/diving_notes"
New-Item -ItemType Directory -Force -Path $outDir | Out-Null

foreach ($line in ($notes -split "`n")) {
    if ([string]::IsNullOrWhiteSpace($line)) { continue }
    $parts = $line -split '\|\|\|', 7
    if ($parts.Count -lt 3) { continue }
    
    $id = $parts[0].Trim()
    $title = $parts[1].Trim()
    $content = $parts[2]
    $createdAt = if ($parts.Count -ge 4) { $parts[3].Trim() } else { "" }
    $kbStatus = if ($parts.Count -ge 5) { $parts[4].Trim() } else { "unknown" }
    $kbVersion = if ($parts.Count -ge 6) { $parts[5].Trim() } else { "0" }
    $sourceType = if ($parts.Count -ge 7) { $parts[6].Trim() } else { "" }
    
    $count++
    Write-Host "  [$count] $title (id=$id, kb=$kbStatus v$kbVersion type=$sourceType)" -ForegroundColor Green
    
    # Save to file
    $safeName = $title -replace '[\\/:*?"<>|]', '_'
    $filePath = Join-Path $outDir "$safeName.md"
    $content | Out-File -FilePath $filePath -Encoding utf8
    Write-Host "    -> $filePath" -ForegroundColor Gray
    Write-Host "    Preview: $($content.Substring(0, [Math]::Min(100, $content.Length)))..." -ForegroundColor Gray
    Write-Host ""
}

Write-Host "Total: $count notes. Files saved to: $outDir" -ForegroundColor Cyan

# Also output as JSONL for eval
$jsonlPath = Join-Path $outDir "..\diving_existing_notes.jsonl"
@"
{"total": $count, "exported_at": "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')"}
"@ | Out-File -FilePath $jsonlPath -Encoding utf8

Write-Host "Manifest: $jsonlPath" -ForegroundColor Cyan

#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Export Diving note metadata through the authenticated API. Full private
  content is written only when -IncludePrivateContent is explicitly supplied.
#>
param(
    [switch]$IncludePrivateContent,
    [string]$DivingPassword = $env:CORTEX_EVAL_DIVING_PASSWORD
)

$ErrorActionPreference = "Stop"
if ([string]::IsNullOrWhiteSpace($DivingPassword)) {
    throw "Set CORTEX_EVAL_DIVING_PASSWORD or pass -DivingPassword."
}

$repositoryDir = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$login = @{username="Diving";password=$DivingPassword} | ConvertTo-Json
$token = (Invoke-RestMethod -Uri "http://127.0.0.1:8000/api/v1/auth/login" -Method POST -ContentType "application/json" -Body $login).token
$headers = @{Authorization="Bearer $token"}
$notes = @()
$page = 1
do {
    $response = Invoke-RestMethod -Uri "http://127.0.0.1:8000/api/v1/notes?page=$page&page_size=100" -Method GET -Headers $headers
    $notes += @($response.items)
    $page++
} while ($notes.Count -lt $response.total)

$outRoot = Join-Path $repositoryDir "backend/testdata/rag"
$manifestPath = Join-Path $outRoot "diving_existing_notes.jsonl"
$notes | ForEach-Object {
    [ordered]@{id=$_.id;title=$_.title;type=$_.type;updated_at=$_.updated_at} | ConvertTo-Json -Compress
} | Set-Content -Path $manifestPath -Encoding utf8

if ($IncludePrivateContent) {
    $contentDir = Join-Path $outRoot "diving_notes"
    New-Item -ItemType Directory -Force -Path $contentDir | Out-Null
    foreach ($note in $notes) {
        $safeName = $note.title -replace '[\\/:*?"<>|]', '_'
        Set-Content -LiteralPath (Join-Path $contentDir "$($note.id)-$safeName.md") -Value $note.content -Encoding utf8
    }
    Write-Host "Exported $($notes.Count) private note bodies to $contentDir" -ForegroundColor Yellow
}

Write-Host "Exported $($notes.Count) note metadata records to $manifestPath" -ForegroundColor Green

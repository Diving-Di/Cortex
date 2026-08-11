#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Quick RAG evaluation runner. Assumes Docker services are running.
  Run this from a PowerShell terminal on your Windows machine:
    cd E:\Codebase\Cortex
    .\backend\scripts\run_eval_now.ps1

  Then upload notes and run evaluation in one step.
#>

param(
    [string]$DivingPassword = $env:CORTEX_EVAL_DIVING_PASSWORD
)

$ErrorActionPreference = "Stop"
$repo = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$testdata = "$repo\backend\testdata\rag"

if ([string]::IsNullOrWhiteSpace($DivingPassword)) {
    throw "Set CORTEX_EVAL_DIVING_PASSWORD or pass -DivingPassword."
}

Write-Host "Cortex RAG Evaluation Quick Runner" -ForegroundColor Cyan
Write-Host "==================================" -ForegroundColor Cyan

# Step 1: Verify Docker
Write-Host "`n[1] Checking Docker services..." -ForegroundColor Yellow
Push-Location $repo
docker compose ps 2>&1 | findstr "backend.*healthy" > $null
if ($LASTEXITCODE -ne 0) {
    Write-Host "  ERROR: Backend is not healthy. Run 'docker compose up -d' first." -ForegroundColor Red
    exit 1
}
Write-Host "  All healthy." -ForegroundColor Green

# Step 2: Login as Diving and upload notes
Write-Host "`n[2] Uploading non-recipe notes..." -ForegroundColor Yellow

$loginBody = @{username="Diving"; password=$DivingPassword} | ConvertTo-Json
$loginResp = Invoke-RestMethod -Uri "http://127.0.0.1:8000/api/v1/auth/token" `
    -Method POST -ContentType "application/json" -Body $loginBody
$token = $loginResp.token
$headers = @{Authorization = "Bearer $token"; "Content-Type" = "application/json"}

$nonRecipeDir = "$testdata\non_recipe_notes"
Get-ChildItem $nonRecipeDir -Filter "*.md" | ForEach-Object {
    $title = [IO.Path]::GetFileNameWithoutExtension($_.Name)
    $content = Get-Content $_.FullName -Raw -Encoding UTF8
    $body = @{title=$title; content=$content; note_date=(Get-Date -Format "yyyy-MM-dd")} | ConvertTo-Json -Depth 1
    $resp = Invoke-RestMethod -Uri "http://127.0.0.1:8000/api/v1/notes" -Method POST -Headers $headers -Body $body
    Write-Host "  Created: $title"
    Invoke-RestMethod -Uri "http://127.0.0.1:8000/api/v1/notes/$($resp.id)/knowledge" `
        -Method PATCH -Headers $headers -Body '{"enabled":true}' | Out-Null
}

# Step 3: Wait for indexing
Write-Host "`n[3] Waiting for indexing (max 5 min)..." -ForegroundColor Yellow
$maxWait = 300
for ($i = 0; $i -lt $maxWait; $i += 5) {
    Start-Sleep -Seconds 5
    $pending = docker compose exec -T db psql -U diary_migrator -d diary_listener -t -A -c `
        "SELECT count(*) FROM knowledge_documents d JOIN tenants t ON t.id = d.tenant_id JOIN users u ON u.id = t.user_id WHERE u.username='Diving' AND d.status NOT IN ('ready','failed') AND d.deleted_at IS NULL;"
    if ($pending.Trim() -eq "0") { break }
    Write-Host "  $pending pending ($($i+5)s)"
}

# Step 4: Run evaluation
Write-Host "`n[4] Running evaluation..." -ForegroundColor Yellow
docker compose build backend 2>&1 | Select-Object -Last 5
New-Item -ItemType Directory -Force -Path "$repo\artifacts\rag-eval" | Out-Null
docker compose run --rm --no-deps --volume "$repo\artifacts\rag-eval:/artifacts" --entrypoint /app/rag-eval backend `
    --dataset "/app/testdata/rag/knowledge_eval_merged.jsonl" `
    --output "/artifacts" --workers 1

# Step 5: Show results
Write-Host "`n[5] Results:" -ForegroundColor Yellow
$latest = Get-ChildItem "$repo\artifacts\rag-eval" -Directory | Sort-Object Name -Descending | Select-Object -First 1
Get-Content "$($latest.FullName)\summary.json" | python3 -c "
import sys, json
s = json.load(sys.stdin)
print(f'Dataset: {s[\"dataset\"]}')
print(f'Cases: {s[\"total\"]}, Succeeded: {s[\"succeeded\"]}, Failed: {s[\"failed\"]}')
print(f'Hit@1: {s[\"metrics\"][\"hit_at_1\"]:.2%}')
print(f'Hit@3: {s[\"metrics\"][\"hit_at_3\"]:.2%}')
print(f'Hit@5: {s[\"metrics\"][\"hit_at_5\"]:.2%}')
print(f'Hit@10: {s[\"metrics\"][\"hit_at_10\"]:.2%}')
print(f'MRR (before rerank): {s[\"metrics\"][\"mrr_before_rerank\"]:.4f}')
print(f'MRR (after rerank): {s[\"metrics\"][\"mrr_after_rerank\"]:.4f}')
rm = s.get('route_metrics', {})
if rm:
    print(f'Vector Hit@10: {rm[\"vector_hit_at_10\"]:.2%}')
    print(f'Fulltext Hit@10: {rm[\"fulltext_hit_at_10\"]:.2%}')
    print(f'Title Hit@10: {rm[\"title_hit_at_10\"]:.2%}')
    print(f'Vector+Fulltext synergy: {rm[\"vector_and_fulltext\"]:.2%}')
"

Pop-Location
Write-Host "`nDone! Results in: $($latest.FullName)" -ForegroundColor Green

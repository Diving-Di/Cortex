#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Run the 164-case retrieval/rerank ablation matrix without generation or LLM judging.
#>
param(
    [ValidateRange(1,4)][int]$Workers = 4,
    [string]$Dataset = "/app/testdata/rag/knowledge_eval_merged.jsonl",
    [string[]]$Routes = @()
)

$ErrorActionPreference = "Stop"
$repoDir = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$outputDir = Join-Path $repoDir "artifacts\rag-eval\ablations"
$runID = Get-Date -Format "yyyyMMdd-HHmmss"
$runRoot = Join-Path $outputDir $runID
New-Item -ItemType Directory -Force -Path $runRoot | Out-Null

$matrix = @(
    @{ Name = "vector";          Vector = 15; Title = 0;  Keyword = 0  },
    @{ Name = "fulltext";        Vector = 0;  Title = 0;  Keyword = 5  },
    @{ Name = "title";           Vector = 0;  Title = 10; Keyword = 0  },
    @{ Name = "vector_fulltext"; Vector = 15; Title = 0;  Keyword = 5  },
    @{ Name = "vector_title";    Vector = 15; Title = 10; Keyword = 0  },
    @{ Name = "fulltext_title";  Vector = 0;  Title = 10; Keyword = 5  },
    @{ Name = "all";             Vector = 15; Title = 10; Keyword = 5  }
)
if ($Routes.Count -gt 0) {
    $wanted = @{}
    foreach ($route in $Routes) { $wanted[$route] = $true }
    $matrix = @($matrix | Where-Object { $wanted.ContainsKey($_.Name) })
    if ($matrix.Count -ne $wanted.Count) {
        throw "Routes contains an unknown or duplicate route name"
    }
}

Push-Location $repoDir
try {
    foreach ($item in $matrix) {
        $hostOutput = Join-Path $runRoot $item.Name
        New-Item -ItemType Directory -Force -Path $hostOutput | Out-Null
        Write-Host "Running $($item.Name)..." -ForegroundColor Cyan
        docker compose run --rm --no-deps `
            --volume "${hostOutput}:/artifacts" `
            --entrypoint /app/rag-eval backend `
            --dataset $Dataset `
            --output /artifacts `
            --workers $Workers `
            --retrieval-only `
            --vector-top-k $item.Vector `
            --title-top-k $item.Title `
            --keyword-top-k $item.Keyword `
            --min-hit-at-10 0
        if ($LASTEXITCODE -ne 0) {
            throw "Ablation $($item.Name) failed with exit code $LASTEXITCODE"
        }
    }
}
finally {
    Pop-Location
}

$rows = foreach ($item in $matrix) {
    $summaryFile = Get-ChildItem (Join-Path $runRoot $item.Name) -Filter summary.json -Recurse |
        Sort-Object LastWriteTime -Descending | Select-Object -First 1
    if (-not $summaryFile) {
        throw "Missing summary for $($item.Name)"
    }
    $summary = Get-Content $summaryFile.FullName -Raw | ConvertFrom-Json
    [pscustomobject]@{
        route = $item.Name
        hit_at_1 = $summary.metrics.hit_at_1
        hit_at_5 = $summary.metrics.hit_at_5
        hit_at_10 = $summary.metrics.hit_at_10
        mrr_before = $summary.metrics.mrr_before_rerank
        mrr_after = $summary.metrics.mrr_after_rerank
        retrieval_p50_ms = $summary.latency_p50.retrieval_ms
        retrieval_p95_ms = $summary.latency_p95.retrieval_ms
    }
}

$metadata = [ordered]@{
    evaluated_at = (Get-Date).ToString("o")
    git_commit = (git rev-parse HEAD).Trim()
    working_tree_dirty = [bool](git status --porcelain)
    dataset = $Dataset
    dataset_sha256 = (Get-FileHash (Join-Path $repoDir "backend\testdata\rag\knowledge_eval_merged.jsonl") -Algorithm SHA256).Hash.ToLowerInvariant()
    case_composition = [ordered]@{ total = 164; historical_recipe = 90; non_recipe = 74 }
    results = $rows
}
$metadata | ConvertTo-Json -Depth 6 | Set-Content (Join-Path $runRoot "ablation-summary.json") -Encoding utf8
$rows | Format-Table -AutoSize
Write-Host "Ablation artifacts: $runRoot" -ForegroundColor Green

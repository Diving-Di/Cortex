#!/usr/bin/env pwsh
<#
.SYNOPSIS
  Cortex RAG 完整评测流水线 — 一键运行
  完成: 上传非菜谱笔记 → 等待索引 → 运行164用例评测 → 对比基线 → 输出分通道分析
.REQUIREMENTS
  Docker Compose 服务已启动且健康
  cd E:\Codebase\Cortex
  .\backend\scripts\run_full_eval.ps1
#>
param(
    [ValidateRange(1,4)][int]$Workers = 1,
    [switch]$SkipUpload,
    [string]$DivingPassword = $env:CORTEX_EVAL_DIVING_PASSWORD
)
$ErrorActionPreference = "Stop"
$repoDir = "E:\Codebase\Cortex"
$testdataDir = "$repoDir\backend\testdata\rag"
$outputDir = "$repoDir\artifacts\rag-eval"
$baselineDir = "$outputDir\20260805-094202"

# ═══════════════════════════════════════════════
# Step 0: Prerequisites
# ═══════════════════════════════════════════════
Write-Host "╔════════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║   Cortex RAG 完整评测流水线               ║" -ForegroundColor Cyan
Write-Host "║   评测集: knowledge_eval_merged.jsonl     ║" -ForegroundColor Cyan
Write-Host "║   用例数: 164 (90菜谱 + 74非菜谱)        ║" -ForegroundColor Cyan
Write-Host "╚════════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host ""

Write-Host "[0/4] 检查前置条件..." -ForegroundColor Yellow
Push-Location $repoDir
try { docker compose ps 2>&1 | Out-Null; if ($LASTEXITCODE -ne 0) { throw "Docker Compose 未运行" } }
finally { Pop-Location }
$ok = docker compose -f "$repoDir\docker-compose.yml" ps backend 2>&1 | Select-String "healthy"
if (-not $ok) { throw "Backend 服务不健康。运行 docker compose up -d 启动所有服务。" }
Write-Host "  所有 Docker 服务健康。" -ForegroundColor Green
docker compose -f "$repoDir\docker-compose.yml" build backend | Select-Object -Last 3
if ($LASTEXITCODE -ne 0) { throw "构建 backend 评测镜像失败" }

# ═══════════════════════════════════════════════
# Step 1: Upload non-recipe notes to Diving
# ═══════════════════════════════════════════════
if (-not $SkipUpload) {
    if ([string]::IsNullOrWhiteSpace($DivingPassword)) {
        throw "Set CORTEX_EVAL_DIVING_PASSWORD or pass -DivingPassword when uploading fixtures."
    }
    Write-Host "[1/4] 上传非菜谱笔记到 Diving 知识库..." -ForegroundColor Yellow
    $login = @{username="Diving";password=$DivingPassword} | ConvertTo-Json
    $token = (Invoke-RestMethod -Uri "http://127.0.0.1:8000/api/v1/auth/token" -Method POST -ContentType "application/json" -Body $login).token
    $headers = @{Authorization="Bearer $token";"Content-Type"="application/json"}
    
    $allNotes = @()
    $page = 1
    do {
        $response = Invoke-RestMethod -Uri "http://127.0.0.1:8000/api/v1/notes?page=$page&page_size=100" -Method GET -Headers $headers
        $allNotes += @($response.items)
        $page++
    } while ($allNotes.Count -lt $response.total)

    Get-ChildItem "$testdataDir\non_recipe_notes" -Filter "*.md" | ForEach-Object {
        $title = [IO.Path]::GetFileNameWithoutExtension($_.Name)
        $content = Get-Content $_.FullName -Raw -Encoding UTF8
        $matches = @($allNotes | Where-Object { $_.title -eq $title })
        if ($matches.Count -gt 1) { throw "Diving 中存在多个同名评测笔记，无法安全更新: $title" }
        if ($matches.Count -eq 1) {
            $note = $matches[0]
            if ($note.content -ne $content) {
                $note = Invoke-RestMethod -Uri "http://127.0.0.1:8000/api/v1/notes/$($note.id)" -Method PATCH -Headers $headers -Body (@{title=$title;content=$content;expected_updated_at=$note.updated_at} | ConvertTo-Json)
                Invoke-RestMethod -Uri "http://127.0.0.1:8000/api/v1/notes/$($note.id)/knowledge" -Method PATCH -Headers $headers -Body '{"enabled":true}' | Out-Null
                Write-Host "  ↻ $title" -ForegroundColor Gray
            } else {
                Write-Host "  = $title" -ForegroundColor DarkGray
            }
        } else {
            $note = Invoke-RestMethod -Uri "http://127.0.0.1:8000/api/v1/notes" -Method POST -Headers $headers -Body (@{title=$title;content=$content;note_date=(Get-Date -Format "yyyy-MM-dd")} | ConvertTo-Json)
            $allNotes += $note
            Invoke-RestMethod -Uri "http://127.0.0.1:8000/api/v1/notes/$($note.id)/knowledge" -Method PATCH -Headers $headers -Body '{"enabled":true}' | Out-Null
            Write-Host "  + $title" -ForegroundColor Gray
        }
    }
}

# ═══════════════════════════════════════════════
# Step 2: Wait for indexing
# ═══════════════════════════════════════════════
Write-Host "[2/4] 等待知识索引完成..." -ForegroundColor Yellow
$waited = 0
do {
    Start-Sleep -Seconds 10; $waited += 10
    docker compose -f "$repoDir\docker-compose.yml" run --rm --no-deps --entrypoint /app/rag-eval backend --dataset "/app/testdata/rag/knowledge_eval_merged.jsonl" --preflight-only 2>&1 | Out-Null
    $ready = ($LASTEXITCODE -eq 0)
    if (-not $ready) { Write-Host "  等待评测文档完成索引 (${waited}s)" -ForegroundColor Gray }
    if ($waited -ge 600 -and -not $ready) { throw "等待评测文档索引超时" }
} while (-not $ready)
Write-Host "  Diving 评测文档预检通过。" -ForegroundColor Green

# ═══════════════════════════════════════════════
# Step 3: Run evaluation
# ═══════════════════════════════════════════════
Write-Host "[3/4] 运行 RAG 评测..." -ForegroundColor Yellow
$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"

New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
Push-Location $repoDir
try {
    Write-Host "  编译并运行评测 (${Workers} worker)..." -ForegroundColor Gray
    docker compose run --rm --no-deps --volume "${outputDir}:/artifacts" --entrypoint /app/rag-eval backend --dataset "/app/testdata/rag/knowledge_eval_merged.jsonl" --output "/artifacts" --workers $Workers --min-hit-at-10 0.99 --min-context-recall 0.80 --min-context-precision 0.85 --min-faithfulness 0.90 --min-answer-relevancy 0.88 --max-failed 0 2>&1
    if ($LASTEXITCODE -ne 0) { throw "评测执行失败，exit code: $LASTEXITCODE" }
}
finally { Pop-Location }

# Find the actual output directory  
$runDir = Get-ChildItem $outputDir -Directory | Sort-Object LastWriteTime -Descending | Select-Object -First 1
if (-not $runDir) { throw "找不到评测输出目录" }
Write-Host "  评测完成: $($runDir.Name)" -ForegroundColor Green

# ═══════════════════════════════════════════════
# Step 4: Analyze results
# ═══════════════════════════════════════════════
Write-Host "`n[4/4] 分析结果..." -ForegroundColor Yellow

# Read current summary
$newSummary = Get-Content "$($runDir.FullName)\summary.json" -Raw | ConvertFrom-Json

Write-Host "`n═══ 核心指标 ═══" -ForegroundColor Cyan
Write-Host "  数据集: $($newSummary.dataset)"
Write-Host "  总用例: $($newSummary.total), 成功: $($newSummary.succeeded), 失败: $($newSummary.failed)"
Write-Host ""

$m = $newSummary.metrics
Write-Host ("  {0,-26} {1,10}" -f "Metric", "Score")
Write-Host ("  {0,-26} {1,10}" -f "-----", "-----")
Write-Host ("  {0,-26} {1,10:P2}" -f "Hit@1", $m.hit_at_1)
Write-Host ("  {0,-26} {1,10:P2}" -f "Hit@3", $m.hit_at_3)
Write-Host ("  {0,-26} {1,10:P2}" -f "Hit@5", $m.hit_at_5)
Write-Host ("  {0,-26} {1,10:P2}" -f "Hit@10", $m.hit_at_10)
Write-Host ("  {0,-26} {1,10:P4}" -f "MRR (rerank前)", $m.mrr_before_rerank)
Write-Host ("  {0,-26} {1,10:P4}" -f "MRR (rerank后)", $m.mrr_after_rerank)
Write-Host ("  {0,-26} {1,10:P2}" -f "Context Recall", $m.context_recall)
Write-Host ("  {0,-26} {1,10:P2}" -f "Context Precision", $m.context_precision)
Write-Host ("  {0,-26} {1,10:P2}" -f "Faithfulness", $m.faithfulness)
Write-Host ("  {0,-26} {1,10:P2}" -f "Answer Relevancy", $m.answer_relevancy)

# Route metrics
$rm = $newSummary.route_metrics
if ($rm) {
    Write-Host "`n═══ 分通道召回命中率 (Hit@10, rerank前) ═══" -ForegroundColor Cyan
    Write-Host ("  {0,-20} {1,8:P2}" -f "向量召回", $rm.vector_hit_at_10)
    Write-Host ("  {0,-20} {1,8:P2}" -f "全文召回", $rm.fulltext_hit_at_10)
    Write-Host ("  {0,-20} {1,8:P2}" -f "标题召回", $rm.title_hit_at_10)
    Write-Host ("  {0,-20} {1,8:P2}" -f "仅向量命中", $rm.vector_only_hit_at_10)
    Write-Host ("  {0,-20} {1,8:P2}" -f "全文增量", $rm.fulltext_incremental)
    Write-Host ("  {0,-20} {1,8:P2}" -f "标题增量", $rm.title_incremental)
    Write-Host ("  {0,-20} {1,8:P2}" -f "向量+全文", $rm.vector_and_fulltext)
    Write-Host ("  {0,-20} {1,8:P2}" -f "向量+标题", $rm.vector_and_title)
    Write-Host ("  {0,-20} {1,8:P2}" -f "三路协同", $rm.all_three)
}

# Compare with baseline
if (Test-Path "$baselineDir\summary.json") {
    $bl = Get-Content "$baselineDir\summary.json" -Raw | ConvertFrom-Json
    Write-Host "`n═══ 与基线对比 (20260805-094202) ═══" -ForegroundColor Cyan
    Write-Host ("  {0,-26} {1,10} {2,10} {3,10}" -f "指标", "基线", "当前", "变化")
    Write-Host ("  {0,-26} {1,10} {2,10} {3,10}" -f "----", "----", "----", "----")
    foreach ($key in @('hit_at_1','hit_at_3','hit_at_5','hit_at_10','mrr_before_rerank','mrr_after_rerank','context_recall','context_precision','faithfulness','answer_relevancy')) {
        $b = $bl.metrics.$key; $n = $m.$key; $d = $n - $b
        $c = if ($d -ge 0) { "Green" } else { "Red" }
        $ds = if ($d -ge 0) { "+$($d.ToString('P2'))" } else { "$($d.ToString('P2'))" }
        Write-Host ("  {0,-26} {1,10} {2,10} " -f $key, $b.ToString('P2'), $n.ToString('P2')) -NoNewline
        Write-Host $ds -ForegroundColor $c
    }
}

# Write comparison report
$report = @"
# RAG 评测对比报告

**时间**: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')
**数据集**: $($newSummary.dataset)
**用例数**: $($newSummary.total)

## 核心指标

| 指标 | 值 |
|------|-----|
| Hit@1 | $($m.hit_at_1.ToString('P2')) |
| Hit@3 | $($m.hit_at_3.ToString('P2')) |
| Hit@5 | $($m.hit_at_5.ToString('P2')) |
| Hit@10 | $($m.hit_at_10.ToString('P2')) |
| MRR (rerank前) | $($m.mrr_before_rerank.ToString('P4')) |
| MRR (rerank后) | $($m.mrr_after_rerank.ToString('P4')) |
| Context Recall | $($m.context_recall.ToString('P2')) |
| Context Precision | $($m.context_precision.ToString('P2')) |
| Faithfulness | $($m.faithfulness.ToString('P2')) |
| Answer Relevancy | $($m.answer_relevancy.ToString('P2')) |
"@

if ($rm) {
    $report += @"

## 分通道召回命中率

| 通道 | Hit@10 |
|------|--------|
| 向量召回 | $($rm.vector_hit_at_10.ToString('P2')) |
| 全文召回 | $($rm.fulltext_hit_at_10.ToString('P2')) |
| 标题召回 | $($rm.title_hit_at_10.ToString('P2')) |
| 仅向量命中 | $($rm.vector_only_hit_at_10.ToString('P2')) |
| 全文增量 | $($rm.fulltext_incremental.ToString('P2')) |
| 标题增量 | $($rm.title_incremental.ToString('P2')) |
| 向量+全文 | $($rm.vector_and_fulltext.ToString('P2')) |
| 向量+标题 | $($rm.vector_and_title.ToString('P2')) |
| 三路协同 | $($rm.all_three.ToString('P2')) |
"@
}

$report | Out-File "$($runDir.FullName)\report.md" -Encoding utf8

Write-Host "`n✓ 评测完成！" -ForegroundColor Green
Write-Host "  输出目录: $($runDir.FullName)" -ForegroundColor White
Write-Host "  报告文件: $($runDir.FullName)\report.md" -ForegroundColor White
Write-Host "  摘要文件: $($runDir.FullName)\summary.json" -ForegroundColor White
Write-Host "  详细结果: $($runDir.FullName)\results.jsonl" -ForegroundColor White

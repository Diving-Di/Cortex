param(
    [string]$Dataset = "/app/testdata/rag/knowledge_eval_v2.jsonl",
    [string]$Output = "artifacts/rag-eval",
    [ValidateRange(1, 4)]
    [int]$Workers = 1,
    [string]$CaseIDs = ""
)

$ErrorActionPreference = "Stop"
$repositoryDir = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$outputDir = Join-Path $repositoryDir $Output
New-Item -ItemType Directory -Force -Path $outputDir | Out-Null
$resolvedOutput = (Resolve-Path $outputDir).Path
$arguments = @("--dataset", $Dataset, "--output", "/artifacts", "--workers", $Workers)
if ($CaseIDs) { $arguments += @("--case-ids", $CaseIDs) }

Push-Location $repositoryDir
try {
    docker compose build backend
    if ($LASTEXITCODE -ne 0) { throw "Failed to build backend evaluation image" }
    docker compose run --rm --no-deps --entrypoint /app/rag-eval `
        --volume "${resolvedOutput}:/artifacts" backend @arguments
    if ($LASTEXITCODE -ne 0) { throw "RAG evaluation failed with exit code $LASTEXITCODE" }
}
finally { Pop-Location }

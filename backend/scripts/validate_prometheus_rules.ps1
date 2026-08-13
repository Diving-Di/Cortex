$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$rules = Join-Path $repoRoot "deploy\prometheus\cortex-alerts.yml"
if (-not (Test-Path -LiteralPath $rules -PathType Leaf)) { throw "alert rules not found" }
docker run --rm --entrypoint /bin/promtool `
    --mount "type=bind,source=$rules,target=/etc/prometheus/cortex-alerts.yml,readonly" `
    prom/prometheus:v3.5.0 check rules /etc/prometheus/cortex-alerts.yml
if ($LASTEXITCODE -ne 0) { throw "promtool validation failed" }

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$rules = Join-Path $repoRoot "deploy\prometheus\cortex-alerts.yml"
$sloRules = Join-Path $repoRoot "deploy\prometheus\cortex-slo.yml"
$config = Join-Path $repoRoot "deploy\prometheus\prometheus.yml"
$alertmanagerConfig = Join-Path $repoRoot "deploy\alertmanager\alertmanager.yml"
foreach ($path in @($rules, $sloRules, $config, $alertmanagerConfig)) {
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Prometheus asset not found: $path" }
}
docker run --rm --entrypoint /bin/promtool `
    --mount "type=bind,source=$rules,target=/etc/prometheus/cortex-alerts.yml,readonly" `
    --mount "type=bind,source=$sloRules,target=/etc/prometheus/cortex-slo.yml,readonly" `
    --mount "type=bind,source=$config,target=/etc/prometheus/prometheus.yml,readonly" `
    prom/prometheus:v3.5.0 check config /etc/prometheus/prometheus.yml
if ($LASTEXITCODE -ne 0) { throw "promtool validation failed" }
docker run --rm --entrypoint /bin/amtool `
    --mount "type=bind,source=$alertmanagerConfig,target=/etc/alertmanager/alertmanager.yml,readonly" `
    prom/alertmanager:v0.28.1 check-config /etc/alertmanager/alertmanager.yml
if ($LASTEXITCODE -ne 0) { throw "Alertmanager config validation failed" }

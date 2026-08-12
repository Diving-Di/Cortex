param(
    [string]$BaseUrl = "http://127.0.0.1:8000",
    [Parameter(Mandatory = $true)][string]$Token,
    [int]$Requests = 1000,
    [int]$Concurrency = 32
)

$ErrorActionPreference = "Stop"
$uri = "$BaseUrl/api/v1/templates/public?ranking=trending&limit=20"
$samples = [System.Collections.Concurrent.ConcurrentBag[double]]::new()
$failures = [System.Collections.Concurrent.ConcurrentBag[string]]::new()
$started = [Diagnostics.Stopwatch]::StartNew()

1..$Requests | ForEach-Object -Parallel {
    $watch = [Diagnostics.Stopwatch]::StartNew()
    try {
        Invoke-WebRequest -UseBasicParsing -Uri $using:uri -Headers @{ Authorization = "Bearer $using:Token" } | Out-Null
        $using:samples.Add($watch.Elapsed.TotalMilliseconds)
    } catch {
        $using:failures.Add($_.Exception.Message)
    }
} -ThrottleLimit $Concurrency

$started.Stop()
$ordered = @($samples | Sort-Object)
if ($ordered.Count -eq 0) { throw "all HTTP requests failed" }
function Percentile([double[]]$values, [double]$p) {
    $index = [Math]::Min($values.Count - 1, [Math]::Ceiling($values.Count * $p) - 1)
    return [Math]::Round($values[$index], 2)
}
[ordered]@{
    requests = $Requests
    succeeded = $ordered.Count
    failed = $failures.Count
    concurrency = $Concurrency
    qps = [Math]::Round($ordered.Count / $started.Elapsed.TotalSeconds, 2)
    p50_ms = Percentile $ordered 0.50
    p95_ms = Percentile $ordered 0.95
    p99_ms = Percentile $ordered 0.99
    wall_seconds = [Math]::Round($started.Elapsed.TotalSeconds, 2)
} | ConvertTo-Json

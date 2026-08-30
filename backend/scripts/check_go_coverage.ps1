param(
    [double]$Minimum = 18
)

$ErrorActionPreference = 'Stop'
$output = Join-Path $PSScriptRoot '..\coverage.out'
try {
    $coverageArgument = "-coverprofile=$output"
    & go test -covermode=atomic $coverageArgument ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
    $functionArgument = "-func=$output"
    $summary = (& go tool cover $functionArgument | Select-Object -Last 1)
    if ($summary -notmatch 'total:\s+\(statements\)\s+([0-9.]+)%') {
        throw "Unable to parse Go coverage summary: $summary"
    }
    $coverage = [double]::Parse($Matches[1], [Globalization.CultureInfo]::InvariantCulture)
    Write-Host "Go statement coverage: $coverage% (minimum $Minimum%)"
    if ($coverage -lt $Minimum) {
        throw "Go statement coverage $coverage% is below the required $Minimum%"
    }
}
finally {
    Remove-Item -LiteralPath $output -Force -ErrorAction SilentlyContinue
}

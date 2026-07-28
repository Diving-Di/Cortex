param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern("^[A-Z][A-Z0-9_]*$")]
    [string]$Name,
    [Parameter(Mandatory = $true)]
    [string]$Value,
    [string]$EnvironmentFile = (Join-Path $PSScriptRoot "..\..\.env")
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($Value) -or $Value.Contains("`r") -or $Value.Contains("`n")) {
    throw "Secret value must be non-empty and must not contain newlines."
}

$path = [System.IO.Path]::GetFullPath($EnvironmentFile)
if (-not [System.IO.File]::Exists($path)) {
    throw "Environment file does not exist: $path"
}

$lines = [System.IO.File]::ReadAllLines($path)
$replacement = "$Name=$Value"
$found = $false
for ($index = 0; $index -lt $lines.Length; $index++) {
    if ($lines[$index] -match "^$([regex]::Escape($Name))=") {
        if ($found) {
            throw "Environment file contains duplicate $Name entries."
        }
        $lines[$index] = $replacement
        $found = $true
    }
}
if (-not $found) {
    $lines += $replacement
}

$temporaryPath = "$path.$([guid]::NewGuid().ToString('N')).tmp"
try {
    [System.IO.File]::WriteAllLines($temporaryPath, $lines, [System.Text.UTF8Encoding]::new($false))
    [System.IO.File]::Move($temporaryPath, $path, $true)
} finally {
    if ([System.IO.File]::Exists($temporaryPath)) {
        [System.IO.File]::Delete($temporaryPath)
    }
}

Write-Output "Updated deployment secret $Name in the ignored environment file."

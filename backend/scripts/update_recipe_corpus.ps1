<#
Update the project-internal howtocook corpus from an external source path.
Usage:
  .\backend\scripts\update_recipe_corpus.ps1 -SourcePath 'E:\Codebase\HowToCook'

This script validates the source and copies `dishes`, `tips`, and `LICENSE` into
`backend/resources/howtocook` in an atomic replace manner.
#>

param(
    [Parameter(Mandatory=$true)]
    [string]$SourcePath,
    [string]$UpstreamUrl = "https://github.com/Anduin2017/HowToCook"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not (Test-Path $SourcePath)) {
    Write-Error "SourcePath does not exist: $SourcePath"
    exit 1
}

$required = @("dishes", "tips", "LICENSE")
foreach ($r in $required) {
    $p = Join-Path $SourcePath $r
    if (-not (Test-Path $p)) {
        Write-Error "Source missing required entry: $r"
        exit 2
    }
}

$repositoryRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..'))
$tmpRoot = Join-Path $repositoryRoot '.recipe-corpus-tmp'
$tmp = Join-Path $tmpRoot ([Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $tmp -Force | Out-Null

try {
    # copy required entries and referenced local resources (images)
    Copy-Item -Path (Join-Path $SourcePath 'dishes') -Destination (Join-Path $tmp 'dishes') -Recurse -Force
    Copy-Item -Path (Join-Path $SourcePath 'tips') -Destination (Join-Path $tmp 'tips') -Recurse -Force
    Copy-Item -Path (Join-Path $SourcePath 'LICENSE') -Destination (Join-Path $tmp 'LICENSE') -Force

    # simple counts
    $mdCount = (Get-ChildItem -Path (Join-Path $tmp 'dishes') -Recurse -Filter *.md | Measure-Object).Count
    $tipsCount = (Get-ChildItem -Path (Join-Path $tmp 'tips') -Recurse -Filter *.md | Measure-Object).Count
    $resCount = ((Get-ChildItem -Path $tmp -Recurse | Where-Object { -not ($_.PSIsContainer) }).Count)

    $upstreamCommit = (& git -C $SourcePath rev-parse HEAD 2>$null)
    if ($LASTEXITCODE -ne 0 -or -not $upstreamCommit) {
        throw "SourcePath must be a Git checkout with a resolvable commit"
    }
    $hashLines = Get-ChildItem -LiteralPath $tmp -Recurse -File |
        Sort-Object FullName |
        ForEach-Object {
            $relative = [System.IO.Path]::GetRelativePath($tmp, $_.FullName).Replace('\', '/')
            $hash = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
            "$relative`t$hash"
        }
    $directoryHashBytes = [System.Text.Encoding]::UTF8.GetBytes(($hashLines -join "`n"))
    $directoryHash = [Convert]::ToHexString(
        [System.Security.Cryptography.SHA256]::HashData($directoryHashBytes)
    ).ToLowerInvariant()

    $sourceJson = [PSCustomObject]@{
        upstream_url = $UpstreamUrl
        upstream_commit = $upstreamCommit.Trim()
        copied_at = (Get-Date).ToString("o")
        markdown_count = ($mdCount + $tipsCount)
        resource_count = $resCount
        directory_sha256 = $directoryHash
    }
    $sourceJson | ConvertTo-Json -Depth 5 | Out-File -FilePath (Join-Path $tmp 'SOURCE.json') -Encoding UTF8

    # atomic replace
    $dest = [System.IO.Path]::GetFullPath((Join-Path $repositoryRoot 'backend\resources\howtocook'))
    New-Item -ItemType Directory -Path (Split-Path -Parent $dest) -Force | Out-Null
    $backup = $null
    if (Test-Path $dest) {
        $backup = $dest + '.bak.' + ([Guid]::NewGuid().ToString())
        Move-Item -LiteralPath $dest -Destination $backup
    }
    Move-Item -LiteralPath $tmp -Destination $dest
    if ($backup) {
        Remove-Item -LiteralPath $backup -Recurse -Force
    }
    Write-Host "Updated howtocook corpus into $dest"
} catch {
    if (Test-Path $tmp) {
        Remove-Item -LiteralPath $tmp -Recurse -Force
    }
    Write-Error "Update failed: $_"
    exit 3
} finally {
    if ((Test-Path $tmpRoot) -and -not (Get-ChildItem -LiteralPath $tmpRoot -Force)) {
        Remove-Item -LiteralPath $tmpRoot -Force
    }
}

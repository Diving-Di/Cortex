$ErrorActionPreference = 'Stop'
$repoDir = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
Push-Location $repoDir
try {
    # Capture resolved configuration without printing credentials.
    $core = docker compose -f docker-compose.light.yml config --format json | ConvertFrom-Json
    if ($LASTEXITCODE -ne 0) { throw 'Light Compose configuration failed' }
    $names = @($core.services.PSObject.Properties.Name | Sort-Object)
    if (($names -join ',') -ne 'backend,db,frontend') { throw 'Light core must contain exactly backend, db and frontend' }
    if ($core.services.db.ports) { throw 'Database must not publish host ports' }
    if ($core.services.backend.environment.STORAGE_BACKEND -ne 'local' -or
        $core.services.backend.environment.EVENT_BUS -ne 'postgres' -or
        $core.services.backend.environment.RAG_RETRIEVAL_BACKEND -ne 'postgres') { throw 'Light backend must use local/PostgreSQL adapters' }
    if ($core.services.backend.volumes[0].source -ne 'light_app_data') { throw 'Light application volume must be isolated' }
    $full = docker compose -f docker-compose.light.yml --profile ai --profile knowledge config --format json | ConvertFrom-Json
    if ($LASTEXITCODE -ne 0) { throw 'Optional profiles configuration failed' }
    if (@($full.services.PSObject.Properties).Count -ne 7) { throw 'Unexpected optional profile services' }
    if ($full.services.'llm-gateway'.ports) { throw 'Gateway must not publish host ports' }
    if ($full.services.'reranker-service'.environment.RERANK_DEVICE -ne 'cpu') { throw 'Light reranker must use CPU' }
    Write-Output 'Light Compose core, optional profiles, internal ports and isolated volumes passed.'
} finally { Pop-Location }

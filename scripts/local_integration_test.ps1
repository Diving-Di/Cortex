# Local integration test script
# Usage: Open PowerShell as admin and run: .\local_integration_test.ps1

$composeFiles = "E:\\Codebase\\Diary-Listener\\docker-compose.yml","E:\\Codebase\\Diary-Listener\\docker-compose.local.yml"
$composeArg = $composeFiles -join " -f "

Write-Host "Bringing up compose stack (local-ai profile)..."
$servicesToStart = @('db','llm-gateway','backend','local-embedding','document-parser')
$dcArgs = @('compose','-f',$composeFiles[0],'-f',$composeFiles[1],'up','-d','--build') + $servicesToStart
& docker @dcArgs

Write-Host "Waiting for services to become healthy (60s)..."
Start-Sleep -Seconds 60

Write-Host "Showing backend logs (last 200 lines)..."
$logsArgs = @('compose','-f',$composeFiles[0],'-f',$composeFiles[1],'--profile','local-ai','logs','--no-color','--tail','200','backend')
& docker @logsArgs | Out-Host

Write-Host "Triggering SyncCorpus via backend admin endpoint (if available)..."
# If the backend exposes an admin trigger endpoint for sync; otherwise run alternative steps
$backendUrl = "http://127.0.0.1:8000"
try {
    $resp = Invoke-RestMethod -Uri "$backendUrl/admin/recipes/sync" -Method Post -Body (@{resources_dir="resources"} | ConvertTo-Json) -ContentType "application/json" -ErrorAction Stop
    Write-Host "Sync trigger response:`n" $resp
} catch {
    Write-Host "Admin sync endpoint not available or failed. Falling back to manual guidance." -ForegroundColor Yellow
    Write-Host "If no admin endpoint exists, run SyncCorpus by executing the backend binary or using the migration scripts."
}

Write-Host "Wait 10s for jobs to be enqueued..."
Start-Sleep -Seconds 10

Write-Host "Tailing backend logs (follow)... Press Ctrl+C to stop"
$tailArgs = @('compose','-f',$composeFiles[0],'-f',$composeFiles[1],'--profile','local-ai','logs','-f','backend')
& docker @tailArgs | Out-Host

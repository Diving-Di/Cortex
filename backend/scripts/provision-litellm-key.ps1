param(
    [string]$GatewayUrl = "http://127.0.0.1:4000",
    [Parameter(Mandatory = $true)]
    [string]$MasterKey,
    [decimal]$MaxBudget = 100,
    [string]$BudgetDuration = "30d",
    [string]$KeyAlias = "diary-listener",
    [string]$EnvironmentFile,
    [switch]$ShowKey
)

$ErrorActionPreference = "Stop"
$headers = @{
    Authorization = "Bearer $MasterKey"
    "Content-Type" = "application/json"
}
$body = @{
    key_alias = $KeyAlias
    models = @("diary-default", "cortex-embedding")
    max_budget = $MaxBudget
    budget_duration = $BudgetDuration
    metadata = @{
        application = "diary-listener"
        managed_by = "backend/scripts/provision-litellm-key.ps1"
    }
} | ConvertTo-Json -Depth 5

$response = Invoke-RestMethod -Method Post -Uri "$($GatewayUrl.TrimEnd('/'))/key/generate" `
    -Headers $headers -Body $body

if ([string]::IsNullOrWhiteSpace($response.key)) {
    throw "LiteLLM did not return a virtual key."
}

if (-not [string]::IsNullOrWhiteSpace($EnvironmentFile)) {
    & (Join-Path $PSScriptRoot "set-env-secret.ps1") -Name "LITELLM_VIRTUAL_KEY" `
        -Value ([string]$response.key) -EnvironmentFile $EnvironmentFile
}
if ($ShowKey) {
    Write-Output "LITELLM_VIRTUAL_KEY=$($response.key)"
} else {
    Write-Output "LiteLLM virtual key provisioned without displaying its value."
}

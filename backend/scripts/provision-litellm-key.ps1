param(
    [string]$GatewayUrl = "http://127.0.0.1:4000",
    [Parameter(Mandatory = $true)]
    [string]$MasterKey,
    [decimal]$MaxBudget = 100,
    [string]$BudgetDuration = "30d",
    [string]$KeyAlias = "diary-listener"
)

$ErrorActionPreference = "Stop"
$headers = @{
    Authorization = "Bearer $MasterKey"
    "Content-Type" = "application/json"
}
$body = @{
    key_alias = $KeyAlias
    models = @("diary-default")
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

Write-Output "LITELLM_VIRTUAL_KEY=$($response.key)"
Write-Output "Store this value in the deployment secret store; it is shown only now."

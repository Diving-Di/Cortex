param(
    [string]$BaseURL = "http://127.0.0.1:8000",
    [int]$ReadyTimeoutSeconds = 180,
    [double]$MinimumRecallAt8 = 1.0,
    [double]$MinimumMRR = 0.8,
    [double]$MinimumNDCG = 0.8,
    [double]$MinimumCitationPrecision = 0.9,
    [double]$MinimumNoAnswerAccuracy = 1.0,
    [long]$MaximumRetrievalP95Ms = 10000,
    [long]$MaximumEndToEndP95Ms = 30000
)

$ErrorActionPreference = "Stop"
$fixtureRoot = Join-Path $PSScriptRoot "..\testdata\knowledge"
$evaluationPath = Join-Path $PSScriptRoot "..\testdata\rag\evaluation.jsonl"

function New-TestTenant {
    param([string]$Name)
    $suffix = [guid]::NewGuid().ToString("N").Substring(0, 10)
    $username = "rag_${Name}_$suffix"
    $password = "correct-horse-battery"
    Invoke-RestMethod "$BaseURL/api/v1/auth/register" -Method Post `
        -ContentType "application/json" -Body (@{
            username = $username
            email = "$username@example.invalid"
            password = $password
        } | ConvertTo-Json) | Out-Null
    $login = Invoke-RestMethod "$BaseURL/api/v1/auth/login" -Method Post `
        -ContentType "application/json" `
        -Body (@{ username = $username; password = $password } | ConvertTo-Json)
    return @{
        Name = $Name
        Token = $login.token
        Headers = @{ Authorization = "Token $($login.token)" }
    }
}

function Get-StatusCode {
    param([scriptblock]$Action)
    try {
        & $Action | Out-Null
        return 200
    } catch {
        if ($_.Exception.Response) {
            return [int]$_.Exception.Response.StatusCode
        }
        throw
    }
}

function Wait-Documents {
    param([hashtable]$Tenant, [int[]]$DocumentIDs)
    $deadline = [DateTime]::UtcNow.AddSeconds($ReadyTimeoutSeconds)
    while ([DateTime]::UtcNow -lt $deadline) {
        $items = Invoke-RestMethod "$BaseURL/api/v1/knowledge/documents?limit=100" `
            -Headers $Tenant.Headers
        $selected = @($items.items | Where-Object { $DocumentIDs -contains $_.id })
        $failed = @($selected | Where-Object { $_.status -eq "failed" })
        if ($failed.Count -gt 0) {
            $codes = @($failed | ForEach-Object { $_.error_code }) -join ","
            throw "Tenant $($Tenant.Name) indexing failed with stable code(s): $codes"
        }
        if ($selected.Count -eq $DocumentIDs.Count -and
            @($selected | Where-Object { $_.status -ne "ready" }).Count -eq 0) {
            return $selected
        }
        Start-Sleep -Seconds 2
    }
    throw "Tenant $($Tenant.Name) documents did not become ready within $ReadyTimeoutSeconds seconds"
}

function Invoke-KnowledgeStream {
    param([hashtable]$Tenant, [string]$Question, [int[]]$DocumentIDs)
    $client = [System.Net.Http.HttpClient]::new()
    $client.Timeout = [TimeSpan]::FromMinutes(5)
    try {
        $request = [System.Net.Http.HttpRequestMessage]::new(
            [System.Net.Http.HttpMethod]::Post,
            "$BaseURL/api/v1/knowledge/chat"
        )
        $request.Headers.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new(
            "Token", $Tenant.Token
        )
        $request.Content = [System.Net.Http.StringContent]::new(
            (@{
                question = $Question
                request_id = [guid]::NewGuid().ToString()
                source_scope = "knowledge"
                document_ids = $DocumentIDs
                collection_ids = @()
            } | ConvertTo-Json -Compress),
            [System.Text.Encoding]::UTF8,
            "application/json"
        )
        $watch = [System.Diagnostics.Stopwatch]::StartNew()
        $response = $client.Send(
            $request,
            [System.Net.Http.HttpCompletionOption]::ResponseHeadersRead
        )
        if (-not $response.IsSuccessStatusCode) {
            return @{
                Status = [int]$response.StatusCode
                Body = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
                RetrievalMs = $null
                TotalMs = $watch.ElapsedMilliseconds
                Sources = @()
                Answer = ""
            }
        }
        $reader = [System.IO.StreamReader]::new(
            $response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
        )
        $eventName = ""
        $sources = @()
        $answer = [System.Text.StringBuilder]::new()
        $retrievalMs = $null
        while (-not $reader.EndOfStream) {
            $line = $reader.ReadLine()
            if ($line.StartsWith("event: ")) {
                $eventName = $line.Substring(7)
                continue
            }
            if (-not $line.StartsWith("data: ")) {
                continue
            }
            $payload = $line.Substring(6) | ConvertFrom-Json
            if ($eventName -eq "retrieval") {
                if ($null -eq $retrievalMs) { $retrievalMs = $watch.ElapsedMilliseconds }
                $sources = @($payload.items)
            } elseif ($eventName -eq "delta" -and $payload.content) {
                [void]$answer.Append([string]$payload.content)
            } elseif ($eventName -eq "sources") {
                $sources = @($payload.items)
            } elseif ($eventName -eq "error") {
                throw "Knowledge SSE error code: $($payload.code)"
            }
        }
        return @{
            Status = 200
            RetrievalMs = $retrievalMs
            TotalMs = $watch.ElapsedMilliseconds
            Sources = $sources
            Answer = $answer.ToString()
        }
    } finally {
        $client.Dispose()
    }
}

function Get-Percentile95 {
    param([long[]]$Values)
    if ($Values.Count -eq 0) { return $null }
    $sorted = @($Values | Sort-Object)
    $index = [Math]::Ceiling($sorted.Count * 0.95) - 1
    return $sorted[[Math]::Max(0, $index)]
}

$tenantA = New-TestTenant "a"
$tenantB = New-TestTenant "b"
$temporaryRoot = Join-Path ([System.IO.Path]::GetTempPath()) "cortex-rag-$([guid]::NewGuid())"
New-Item -ItemType Directory -Path "$temporaryRoot\a","$temporaryRoot\b" | Out-Null
try {
    $documents = @{ A = @(); B = @() }
    foreach ($extension in @("txt", "pdf", "docx")) {
        Copy-Item (Join-Path $fixtureRoot "tenant-a.$extension") "$temporaryRoot\a\shared.$extension"
        Copy-Item (Join-Path $fixtureRoot "tenant-b.$extension") "$temporaryRoot\b\shared.$extension"
        $documents.A += Invoke-RestMethod "$BaseURL/api/v1/knowledge/documents" -Method Post `
            -Headers $tenantA.Headers -Form @{ file = (Get-Item "$temporaryRoot\a\shared.$extension") }
        $documents.B += Invoke-RestMethod "$BaseURL/api/v1/knowledge/documents" -Method Post `
            -Headers $tenantB.Headers -Form @{ file = (Get-Item "$temporaryRoot\b\shared.$extension") }
    }

    $idsA = @($documents.A | ForEach-Object { [int]$_.id })
    $idsB = @($documents.B | ForEach-Object { [int]$_.id })
    $readyA = Wait-Documents $tenantA $idsA
    $readyB = Wait-Documents $tenantB $idsB

    $crossStatuses = @()
    foreach ($id in $idsA) {
        $crossStatuses += Get-StatusCode {
            Invoke-WebRequest "$BaseURL/api/v1/knowledge/documents/$id" -Headers $tenantB.Headers `
                -ErrorAction Stop
        }
        $crossStatuses += Get-StatusCode {
            Invoke-WebRequest "$BaseURL/api/v1/knowledge/documents/$id/download" `
                -Headers $tenantB.Headers -ErrorAction Stop
        }
        $crossStatuses += Get-StatusCode {
            Invoke-WebRequest "$BaseURL/api/v1/knowledge/documents/$id" -Method Delete `
                -Headers $tenantB.Headers -ErrorAction Stop
        }
    }
    if (@($crossStatuses | Where-Object { $_ -ne 404 }).Count -gt 0) {
        throw "Cross-tenant knowledge access did not consistently return 404: $crossStatuses"
    }

    $evaluation = Get-Content $evaluationPath | Where-Object { $_.Trim() } | ConvertFrom-Json
    $warmup = Invoke-KnowledgeStream $tenantA $evaluation[0].question $idsA
    if ($warmup.Status -ne 200) {
        throw "Knowledge evaluation warm-up failed with status $($warmup.Status)."
    }
    $runs = @()
    foreach ($case in $evaluation) {
        $result = Invoke-KnowledgeStream $tenantA $case.question $idsA
        $runs += [pscustomobject]@{
            ID = $case.id
            Answerable = $case.answerable
            Status = $result.Status
            ExpectedDocument = $case.expected_document
            ExpectedPhrase = $case.expected_phrase
            Sources = @($result.Sources)
            Answer = $result.Answer
            RetrievalMs = $result.RetrievalMs
            TotalMs = $result.TotalMs
        }
    }

    $answerable = @($runs | Where-Object { $_.Answerable })
    $answerableFailures = @($answerable | Where-Object { $_.Status -ne 200 })
    if ($answerableFailures.Count -gt 0) {
        $values = @($answerableFailures | ForEach-Object { "$($_.ID):$($_.Status)" }) -join ","
        throw "Answerable evaluation requests failed: $values"
    }
    $recallHits = 0
    $reciprocalRanks = @()
    $dcgValues = @()
    $citationHits = 0
    $citationTotal = 0
    foreach ($run in $answerable) {
        $names = @($run.Sources | ForEach-Object { $_.title })
        $rank = [Array]::IndexOf($names, $run.ExpectedDocument) + 1
        if ($rank -gt 0 -and $rank -le 8) { $recallHits++ }
        $reciprocalRanks += if ($rank -gt 0) { 1.0 / $rank } else { 0.0 }
        $dcgValues += if ($rank -gt 0) { 1.0 / [Math]::Log($rank + 1, 2) } else { 0.0 }
        if (-not $run.Answer.Contains($run.ExpectedPhrase)) {
            throw "Answer $($run.ID) did not contain expected synthetic fact."
        }
        if ($run.Answer -match "琥珀九号|Amber Nine|2049-09-23|2049年9月23日|暮光工作室|Twilight Studio") {
            throw "Answer $($run.ID) leaked the other tenant's synthetic fact."
        }
        $citedRanks = @(
            [regex]::Matches($run.Answer, '\[K(\d+)\]') |
                ForEach-Object { [int]$_.Groups[1].Value } |
                Sort-Object -Unique
        )
        foreach ($citedRank in $citedRanks) {
            $citationTotal++
            if ($citedRank -ge 1 -and $citedRank -le $names.Count -and
                $names[$citedRank - 1] -eq $run.ExpectedDocument) {
                $citationHits++
            }
        }
    }
    $noAnswer = @($runs | Where-Object { -not $_.Answerable })
    $noAnswerCorrect = @($noAnswer | Where-Object { $_.Status -eq 404 }).Count
    if ($noAnswerCorrect -ne $noAnswer.Count) {
        throw "No-answer evaluation did not consistently return 404."
    }

    $racePath = Join-Path $temporaryRoot "a\delete-during-index.txt"
    $raceLine = "并发删除验收资料：worker 完成旧索引时不得重新激活已经删除的文档。`n"
    [System.IO.File]::WriteAllText(
        $racePath,
        ($raceLine * 30000),
        [System.Text.UTF8Encoding]::new($false)
    )
    $raceDocument = Invoke-RestMethod "$BaseURL/api/v1/knowledge/documents" -Method Post `
        -Headers $tenantA.Headers -Form @{ file = (Get-Item $racePath) }
    Start-Sleep -Seconds 6
    Invoke-WebRequest "$BaseURL/api/v1/knowledge/documents/$($raceDocument.id)" -Method Delete `
        -Headers $tenantA.Headers | Out-Null
    Start-Sleep -Seconds 15
    $staleWorkerStatus = Get-StatusCode {
        Invoke-WebRequest "$BaseURL/api/v1/knowledge/documents/$($raceDocument.id)" `
            -Headers $tenantA.Headers -ErrorAction Stop
    }
    if ($staleWorkerStatus -ne 404) {
        throw "A stale index worker reactivated a deleted document; status=$staleWorkerStatus"
    }

    foreach ($id in $idsA) {
        Invoke-WebRequest "$BaseURL/api/v1/knowledge/documents/$id" -Method Delete `
            -Headers $tenantA.Headers | Out-Null
    }
    $postDelete = Invoke-KnowledgeStream $tenantA "苍穹计划的发布口令是什么？" @()
    if ($postDelete.Status -ne 404) {
        throw "Deleted documents remained retrievable; status=$($postDelete.Status)"
    }

    $metrics = [pscustomobject]@{
        TenantAReady = @($readyA).Count
        TenantBReady = @($readyB).Count
        CrossTenant404Checks = @($crossStatuses).Count
        RecallAt8 = if ($answerable.Count) { $recallHits / $answerable.Count } else { 0 }
        MRR = if ($reciprocalRanks.Count) {
            ($reciprocalRanks | Measure-Object -Average).Average
        } else { 0 }
        NDCG = if ($dcgValues.Count) { ($dcgValues | Measure-Object -Average).Average } else { 0 }
        CitationPrecision = if ($citationTotal) { $citationHits / $citationTotal } else { 0 }
        NoAnswerAccuracy = if ($noAnswer.Count) { $noAnswerCorrect / $noAnswer.Count } else { 0 }
        RetrievalP95Ms = Get-Percentile95 @($runs.RetrievalMs | Where-Object { $null -ne $_ })
        EndToEndP95Ms = Get-Percentile95 @($runs.TotalMs)
        PostDeleteStatus = $postDelete.Status
        StaleWorkerStatus = $staleWorkerStatus
    }
    if ($metrics.RecallAt8 -lt $MinimumRecallAt8 -or
        $metrics.MRR -lt $MinimumMRR -or
        $metrics.NDCG -lt $MinimumNDCG -or
        $metrics.CitationPrecision -lt $MinimumCitationPrecision -or
        $metrics.NoAnswerAccuracy -lt $MinimumNoAnswerAccuracy -or
        $metrics.RetrievalP95Ms -gt $MaximumRetrievalP95Ms -or
        $metrics.EndToEndP95Ms -gt $MaximumEndToEndP95Ms) {
        throw "Knowledge quality or latency gate failed: $($metrics | ConvertTo-Json -Compress)"
    }
    $metrics | ConvertTo-Json -Compress
} finally {
    Remove-Item -LiteralPath $temporaryRoot -Recurse -Force -ErrorAction SilentlyContinue
}

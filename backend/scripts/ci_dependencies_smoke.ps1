$ErrorActionPreference = 'Stop'

$minio = Invoke-WebRequest -UseBasicParsing http://127.0.0.1:59000/minio/health/ready
if ($minio.StatusCode -ne 200) { throw 'MinIO is not ready' }

$topics = Invoke-RestMethod http://127.0.0.1:58082/topics
if ($topics -notcontains 'cortex.integration.v1') { throw 'Kafka integration topic is missing' }

$credentials = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes('elastic:ci-elasticsearch-password'))
$cluster = Invoke-RestMethod http://127.0.0.1:59200/_cluster/health -Headers @{ Authorization = "Basic $credentials" }
if ($cluster.status -notin @('green', 'yellow')) { throw "Elasticsearch status is $($cluster.status)" }

Write-Host 'MinIO, Kafka REST, and Elasticsearch integration dependencies are ready.'

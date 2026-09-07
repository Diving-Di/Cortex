# Dot-source this script before running Go integration tests on this machine.
# These credentials belong exclusively to docker-compose.ci.yml.
$env:DATABASE_URL = 'postgresql://cortex_app:ci-app-password@127.0.0.1:55432/cortex?sslmode=disable'
$env:MIGRATION_DATABASE_URL = 'postgresql://cortex_migrator:ci-migrator-password@127.0.0.1:55432/cortex?sslmode=disable'
$env:REDIS_TEST_URL = 'redis://default:ci-redis-password@127.0.0.1:56379/0'
$env:MINIO_TEST_ENDPOINT = 'http://127.0.0.1:59000'
$env:MINIO_TEST_ACCESS_KEY = 'ci-minio-root'
$env:MINIO_TEST_SECRET_KEY = 'ci-minio-password'

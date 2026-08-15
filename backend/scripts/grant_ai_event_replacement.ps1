param(
    [Parameter(Mandatory = $true)]
    [guid]$EventID,
    [ValidateRange(1, 1000)]
    [int]$Count = 1,
    [ValidateRange(0, 1440)]
    [int]$ExtendMinutes = 10,
    [switch]$ConfirmReplacement
)

$ErrorActionPreference = "Stop"
if (-not $ConfirmReplacement) {
    throw "Refusing to grant replacement slots without -ConfirmReplacement"
}

$sql = @'
BEGIN;
WITH updated AS (
    UPDATE ai_flash_events
    SET total_slots = total_slots + :replacement_count,
        closes_at = GREATEST(closes_at, now()) + make_interval(mins => :extend_minutes),
        status = CASE WHEN status = 'cancelled' THEN status ELSE 'scheduled' END,
        reservation_ready = false,
        updated_at = now()
    WHERE public_id = :'event_id'::uuid
      AND status <> 'cancelled'
    RETURNING public_id, total_slots, claimed_slots, closes_at
), recorded AS (
    INSERT INTO outbox_events(id, aggregate_type, aggregate_id, event_type, payload)
    SELECT gen_random_uuid(), 'ai_flash_event', public_id::text,
           'ai_flash_event.replacement_granted',
           jsonb_build_object('count', :replacement_count, 'extend_minutes', :extend_minutes)
    FROM updated
    RETURNING aggregate_id
)
SELECT u.public_id, u.total_slots, u.claimed_slots, u.closes_at
FROM updated u JOIN recorded r ON r.aggregate_id = u.public_id::text;
COMMIT;
'@

$result = $sql | docker compose exec -T db psql -U cortex_migrator -d cortex `
    -v ON_ERROR_STOP=1 `
    -v "event_id=$EventID" `
    -v "replacement_count=$Count" `
    -v "extend_minutes=$ExtendMinutes" `
    --csv
if ($LASTEXITCODE -ne 0) {
    throw "Failed to grant replacement slots"
}
if (($result | Select-String -SimpleMatch $EventID.ToString()).Count -ne 1) {
    throw "Event was not found or is cancelled; no replacement slot was granted"
}
$result

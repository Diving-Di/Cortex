package store

import (
	"context"

	"github.com/google/uuid"
)

func (s *Store) ConsumerSucceeded(ctx context.Context, group string, eventID uuid.UUID) (bool, error) {
	var succeeded bool
	err := s.AdminPool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM consumer_receipts WHERE consumer_group=$1 AND event_id=$2 AND result_code='success')`, group, eventID).Scan(&succeeded)
	return succeeded, err
}

func (s *Store) ConsumerReceived(ctx context.Context, group string, eventID uuid.UUID) (bool, error) {
	tag, err := s.AdminPool.Exec(ctx, `INSERT INTO consumer_receipts(consumer_group,event_id,result_code) VALUES($1,$2,'processing') ON CONFLICT DO NOTHING`, group, eventID)
	return err == nil && tag.RowsAffected() == 1, err
}
func (s *Store) FinishConsumerReceipt(ctx context.Context, group string, eventID uuid.UUID, code string) error {
	_, err := s.AdminPool.Exec(ctx, `UPDATE consumer_receipts SET result_code=$3,processed_at=now() WHERE consumer_group=$1 AND event_id=$2`, group, eventID, code)
	return err
}
func (s *Store) DeadLetter(ctx context.Context, group, topic string, eventID uuid.UUID, code string, attempt int) error {
	_, err := s.AdminPool.Exec(ctx, `INSERT INTO event_dead_letters(event_id,consumer_group,topic,error_code,attempt_count) VALUES($1,$2,$3,$4,$5) ON CONFLICT(event_id,consumer_group) DO UPDATE SET error_code=excluded.error_code,attempt_count=excluded.attempt_count`, eventID, group, topic, code, attempt)
	return err
}

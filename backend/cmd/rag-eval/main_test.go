package main

import (
	"context"
	"errors"
	"testing"
)

func TestWithRetryRecoversFromTransientFailure(t *testing.T) {
	attempts := 0
	value, err := withRetry(context.Background(), 3, 0, func() (string, error) {
		attempts++
		if attempts < 3 {
			return "", errors.New("transient")
		}
		return "ok", nil
	})
	if err != nil || value != "ok" || attempts != 3 {
		t.Fatalf("value=%q attempts=%d err=%v", value, attempts, err)
	}
}

func TestWithRetryStopsAfterLimit(t *testing.T) {
	attempts := 0
	_, err := withRetry(context.Background(), 2, 0, func() (string, error) {
		attempts++
		return "", errors.New("persistent")
	})
	if err == nil || attempts != 2 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
}

package server

import (
    "testing"
    "time"
)

func TestNormalizePeriod(t *testing.T) {
    input, _ := time.Parse(time.DateOnly, "2026-07-22")
    weekly := normalizePeriod("weekly", &input)
    if got := weekly.Format(time.DateOnly); got != "2026-07-20" {
        t.Fatalf("weekly date = %s", got)
    }
    monthly := normalizePeriod("monthly", &input)
    if got := monthly.Format(time.DateOnly); got != "2026-07-01" {
        t.Fatalf("monthly date = %s", got)
    }
}

func TestListPaginationValidation(t *testing.T) {
    if _, err := positiveQueryInt("0", 20, 100); err == nil {
        t.Fatal("zero page size was accepted")
    }
    if _, err := positiveQueryInt("101", 20, 100); err == nil {
        t.Fatal("oversized page size was accepted")
    }
    if value, err := positiveQueryInt("", 20, 100); err != nil || value != 20 {
        t.Fatal("default page size was not applied")
    }
}

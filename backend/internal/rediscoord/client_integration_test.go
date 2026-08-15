package rediscoord

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRedisCoordinationIntegration(t *testing.T) {
	raw := os.Getenv("REDIS_TEST_URL")
	if raw == "" {
		t.Skip("REDIS_TEST_URL not set")
	}
	c, err := New(raw)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	suffix := time.Now().Format("150405.000000000")
	stock, claimed := "test:{"+suffix+"}:stock", "test:{"+suffix+"}:claimed"
	preparedStock, preparedClaimed, preparedWindow := "prepared:{"+suffix+"}:stock", "prepared:{"+suffix+"}:claimed", "prepared:{"+suffix+"}:window"
	preparedEligible, preparedPoints := "prepared:{"+suffix+"}:eligible", "prepared:{"+suffix+"}:points"
	preparedPending := "prepared:{" + suffix + "}:pending"
	versionStable := AIEventKeys("integration-"+suffix, "")
	versionOne := AIEventKeys("integration-"+suffix, "v1")
	versionTwo := AIEventKeys("integration-"+suffix, "v2")
	loadStock, loadClaimed, loadWindow := "load:{"+suffix+"}:stock", "load:{"+suffix+"}:claimed", "load:{"+suffix+"}:window"
	loadEligible, loadPoints := "load:{"+suffix+"}:eligible", "load:{"+suffix+"}:points"
	loadPending := "load:{" + suffix + "}:pending"
	defer func() {
		keys := append(versionOne.DataKeys(), versionTwo.DataKeys()...)
		keys = append(keys, versionStable.ActiveVersion, versionStable.BuildLock)
		keys = append(keys, stock, claimed, preparedStock, preparedClaimed, preparedWindow, preparedEligible, preparedPoints, preparedPending, loadStock, loadClaimed, loadWindow, loadEligible, loadPoints, loadPending, "test:cache:"+suffix, "test:rate:"+suffix, "cortex:outbox:processed:event-"+suffix, "cortex:outbox:processed:projection-"+suffix, "cortex:outbox:processed:projection-next-"+suffix)
		_ = c.Delete(ctx, keys...)
		_, _ = c.commands(ctx, []string{"ZREM", "cortex:tpl:rank:trending", "public-" + suffix})
	}()
	var accepted atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			result, e := c.Reserve(ctx, stock, claimed, suffix+string(rune(i+1000)), 10, time.Minute)
			if e != nil {
				t.Errorf("reserve: %v", e)
				return
			}
			if result == 1 {
				accepted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if accepted.Load() != 10 {
		t.Fatalf("accepted=%d", accepted.Load())
	}
	eligible := map[string]int64{"a": 200, "b": 100, "c": 99}
	if got, err := c.ReservePrepared(ctx, preparedStock, preparedClaimed, preparedWindow, preparedEligible, preparedPoints, preparedPending, "a"); err != nil || got != -1 {
		t.Fatalf("unprepared got=%d err=%v", got, err)
	}
	if err := c.WarmEvent(ctx, preparedStock, preparedClaimed, preparedWindow, preparedEligible, preparedPoints, preparedPending, time.Now().Add(time.Minute), time.Now().Add(2*time.Minute), 2, 100, nil, eligible, time.Minute); err != nil {
		t.Fatal(err)
	}
	if got, err := c.ReservePrepared(ctx, preparedStock, preparedClaimed, preparedWindow, preparedEligible, preparedPoints, preparedPending, "a"); err != nil || got != -2 {
		t.Fatalf("not open got=%d err=%v", got, err)
	}
	if err := c.WarmEvent(ctx, preparedStock, preparedClaimed, preparedWindow, preparedEligible, preparedPoints, preparedPending, time.Now().Add(-2*time.Minute), time.Now().Add(-time.Minute), 2, 100, nil, eligible, time.Minute); err != nil {
		t.Fatal(err)
	}
	if got, err := c.ReservePrepared(ctx, preparedStock, preparedClaimed, preparedWindow, preparedEligible, preparedPoints, preparedPending, "a"); err != nil || got != -3 {
		t.Fatalf("closed got=%d err=%v", got, err)
	}
	if err := c.WarmEvent(ctx, preparedStock, preparedClaimed, preparedWindow, preparedEligible, preparedPoints, preparedPending, time.Now().Add(-time.Minute), time.Now().Add(time.Minute), 3, 100, nil, eligible, time.Minute); err != nil {
		t.Fatal(err)
	}
	locked, err := c.AcquireAIEventBuildLock(ctx, versionStable.BuildLock, "owner-1", time.Minute)
	if err != nil || !locked {
		t.Fatalf("build lock locked=%v err=%v", locked, err)
	}
	if err = c.BuildAIEventVersion(ctx, versionOne, time.Now().Add(-time.Minute), time.Now().Add(time.Minute), 2, 100, nil, eligible, time.Minute, 2); err != nil {
		t.Fatal(err)
	}
	if switched, e := c.SwitchAIEventVersion(ctx, versionStable, "", "v1", "wrong-owner"); e != nil || switched != -1 {
		t.Fatalf("wrong owner switch=%d err=%v", switched, e)
	}
	if switched, e := c.SwitchAIEventVersion(ctx, versionStable, "", "v1", "owner-1"); e != nil || switched != 1 {
		t.Fatalf("switch=%d err=%v", switched, e)
	}
	if got, e := c.ReservePreparedVersioned(ctx, versionOne, "stale", "a"); e != nil || got != VersionChanged {
		t.Fatalf("stale reserve=%d err=%v", got, e)
	}
	if got, e := c.ReservePreparedVersioned(ctx, versionOne, "v1", "a"); e != nil || got != 1 {
		t.Fatalf("version reserve=%d err=%v", got, e)
	}
	if deleted, e := c.CleanupAIEventVersion(ctx, versionOne, "v1"); e != nil || deleted {
		t.Fatalf("active cleanup deleted=%v err=%v", deleted, e)
	}
	if err = c.BuildAIEventVersion(ctx, versionTwo, time.Now().Add(-time.Minute), time.Now().Add(time.Minute), 2, 100, nil, eligible, time.Minute, 2); err != nil {
		t.Fatal(err)
	}
	if switched, e := c.SwitchAIEventVersion(ctx, versionStable, "v1", "v2", "owner-1"); e != nil || switched != 1 {
		t.Fatalf("second switch=%d err=%v", switched, e)
	}
	if deleted, e := c.CleanupAIEventVersion(ctx, versionOne, "v1"); e != nil || deleted {
		t.Fatalf("pending cleanup deleted=%v err=%v", deleted, e)
	}
	if err = c.ConfirmReservation(ctx, versionOne.Pending, "a"); err != nil {
		t.Fatal(err)
	}
	if deleted, e := c.CleanupAIEventVersion(ctx, versionOne, "v1"); e != nil || !deleted {
		t.Fatalf("settled cleanup deleted=%v err=%v", deleted, e)
	}
	for i, want := range []int{1, 1, 1, -4} {
		got, err := c.ReservePrepared(ctx, preparedStock, preparedClaimed, preparedWindow, preparedEligible, preparedPoints, preparedPending, string(rune('a'+i)))
		if err != nil || got != want {
			t.Fatalf("prepared %d got=%d want=%d err=%v", i, got, want, err)
		}
	}
	if got, err := c.ReservePrepared(ctx, preparedStock, preparedClaimed, preparedWindow, preparedEligible, preparedPoints, preparedPending, "a"); err != nil || got != 2 {
		t.Fatalf("duplicate got=%d err=%v", got, err)
	}
	if err := c.Compensate(ctx, preparedStock, preparedClaimed, preparedWindow, preparedPoints, preparedPending, "a"); err != nil {
		t.Fatal(err)
	}
	if got, err := c.ReservePrepared(ctx, preparedStock, preparedClaimed, preparedWindow, preparedEligible, preparedPoints, preparedPending, "a"); err != nil || got != 1 {
		t.Fatalf("compensated reserve got=%d err=%v", got, err)
	}
	if err := c.ConfirmReservation(ctx, preparedPending, "a"); err != nil {
		t.Fatal(err)
	}
	if err := c.ConfirmReservation(ctx, preparedPending, "b"); err != nil {
		t.Fatal(err)
	}
	if err := c.ConfirmReservation(ctx, preparedPending, "c"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.commands(ctx, []string{"ZADD", preparedPending, strconv.FormatInt(time.Now().Add(-31*time.Second).Unix(), 10), "orphan"}); err != nil {
		t.Fatal(err)
	}
	if err := c.WarmEvent(ctx, preparedStock, preparedClaimed, preparedWindow, preparedEligible, preparedPoints, preparedPending, time.Now().Add(-time.Minute), time.Now().Add(time.Minute), 2, 100, nil, eligible, time.Minute); err != nil {
		t.Fatal(err)
	}
	if value, ok, err := c.Get(ctx, preparedStock); err != nil || !ok || value != "2" {
		t.Fatalf("orphan reconciliation stock=%q ok=%v err=%v", value, ok, err)
	}
	loadUsers := make(map[string]int64, 10000)
	for i := 0; i < 10000; i++ {
		loadUsers[fmt.Sprintf("user-%05d", i)] = 100
	}
	if err := c.WarmEvent(ctx, loadStock, loadClaimed, loadWindow, loadEligible, loadPoints, loadPending, time.Now().Add(-time.Minute), time.Now().Add(time.Minute), 10, 100, nil, loadUsers, time.Minute); err != nil {
		t.Fatal(err)
	}
	accepted.Store(0)
	started := time.Now()
	gate := make(chan struct{}, 512)
	for i := 0; i < 10000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			gate <- struct{}{}
			defer func() { <-gate }()
			result, e := c.ReservePrepared(ctx, loadStock, loadClaimed, loadWindow, loadEligible, loadPoints, loadPending, fmt.Sprintf("user-%05d", i))
			if e != nil {
				t.Errorf("load reserve: %v", e)
				return
			}
			if result == 1 {
				accepted.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if accepted.Load() != 10 {
		t.Fatalf("load accepted=%d", accepted.Load())
	}
	if elapsed := time.Since(started); elapsed > 15*time.Second {
		t.Fatalf("10000 reservations took %s", elapsed)
	}
	if err := c.Set(ctx, "test:cache:"+suffix, "value", time.Minute); err != nil {
		t.Fatal(err)
	}
	if value, ok, err := c.Get(ctx, "test:cache:"+suffix); err != nil || !ok || value != "value" {
		t.Fatalf("cache value=%q ok=%v err=%v", value, ok, err)
	}
	event := "event-" + suffix
	publicID := "public-" + suffix
	if err := c.RebuildTemplateRankings(ctx, []RankingProjection{{PublicID: publicID, PublishedAt: time.Now(), TrendingScore: 3}}); err != nil {
		t.Fatal(err)
	}
	if err := c.ApplyTemplateEvent(ctx, event, publicID, "template.like", "", 1, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := c.ApplyTemplateEvent(ctx, event, publicID, "template.like", "", 1, time.Now()); err != nil {
		t.Fatal(err)
	}
	trendingKey, ok, err := c.ActiveTemplateRankingKey(ctx, "trending", "")
	if err != nil || !ok {
		t.Fatalf("active ranking key ok=%v err=%v", ok, err)
	}
	if score, ok, err := c.Score(ctx, trendingKey, publicID); err != nil || !ok || score != 3 {
		t.Fatalf("idempotent score=%v ok=%v err=%v", score, ok, err)
	}
	projectionID := "projection-" + suffix
	if err := c.ApplyTemplateProjection(ctx, projectionID, publicID, "template.like", "", time.Now(), 17, 4); err != nil {
		t.Fatal(err)
	}
	if err := c.ApplyTemplateProjection(ctx, projectionID, publicID, "template.like", "", time.Now(), 99, 4); err != nil {
		t.Fatal(err)
	}
	if score, ok, err := c.Score(ctx, trendingKey, publicID); err != nil || !ok || score != 17 {
		t.Fatalf("absolute idempotent score=%v ok=%v err=%v", score, ok, err)
	}
	if err := c.ApplyTemplateProjection(ctx, "projection-next-"+suffix, publicID, "template.favorite", "", time.Now(), 17, 4); err != nil {
		t.Fatal(err)
	}
	if score, ok, err := c.Score(ctx, trendingKey, publicID); err != nil || !ok || score != 17 {
		t.Fatalf("absolute replay score=%v ok=%v err=%v", score, ok, err)
	}
	if items, err := c.RankingPage(ctx, trendingKey, nil, 10); err != nil || len(items) == 0 {
		t.Fatalf("ranking items=%v err=%v", items, err)
	}
	if uv, err := c.TemplateUniqueVisitors(ctx, publicID, time.Now().In(time.FixedZone("CST", 8*3600)).Format("20060102")); err != nil || uv != 0 {
		t.Fatalf("unexpected UV %d err=%v", uv, err)
	}
	for i := 0; i < 3; i++ {
		allowed, err := c.Allow(ctx, "test:rate:"+suffix, 2, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if allowed != (i < 2) {
			t.Fatalf("rate attempt %d allowed=%v", i, allowed)
		}
	}
}

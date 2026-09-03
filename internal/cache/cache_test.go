package cache

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func fixed(body string, policy Policy, tags ...string) Loader {
	return func(bool) (Value, Policy, error) {
		return Value{Body: []byte(body), Tags: tags}, policy, nil
	}
}

func newCache(limit int64) (*Cache, *clock) {
	c := &clock{now: time.Unix(1700000000, 0)}
	return New(Options{Limit: limit, Clock: c.Now}), c
}

func do(t *testing.T, cache *Cache, key string, load Loader) (string, Status) {
	t.Helper()
	value, status, err := cache.Do(key, load)
	if err != nil {
		t.Fatalf("Do(%q): %v", key, err)
	}
	return string(value.Body), status
}

func TestSecondReadIsAHit(t *testing.T) {
	cache, _ := newCache(1 << 20)
	load := fixed("page", Policy{TTL: time.Minute})
	if body, status := do(t, cache, "a", load); body != "page" || status != StatusMiss {
		t.Errorf("body = %q, status = %v", body, status)
	}
	if body, status := do(t, cache, "a", load); body != "page" || status != StatusHit {
		t.Errorf("body = %q, status = %v", body, status)
	}
}

func TestAnExpiredEntryIsAMissAgain(t *testing.T) {
	cache, tick := newCache(1 << 20)
	load := fixed("page", Policy{TTL: time.Minute})
	do(t, cache, "a", load)
	tick.Advance(2 * time.Minute)
	if _, status := do(t, cache, "a", load); status != StatusMiss {
		t.Errorf("status = %v, want a miss", status)
	}
}

func TestAStaleEntryIsServedAndRefreshed(t *testing.T) {
	cache, tick := newCache(1 << 20)
	var calls atomic.Int64
	load := func(bool) (Value, Policy, error) {
		calls.Add(1)
		return Value{Body: []byte(fmt.Sprintf("page %d", calls.Load()))},
			Policy{TTL: time.Minute, Stale: time.Hour}, nil
	}
	do(t, cache, "a", load)
	tick.Advance(2 * time.Minute)

	body, status := do(t, cache, "a", load)
	if status != StatusStale || body != "page 1" {
		t.Fatalf("body = %q, status = %v", body, status)
	}
	cache.Wait()
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want a background refresh", calls.Load())
	}
	if body, status := do(t, cache, "a", load); status != StatusHit || body != "page 2" {
		t.Errorf("body = %q, status = %v", body, status)
	}
}

func TestAnEntryBeyondTheStaleWindowIsDropped(t *testing.T) {
	cache, tick := newCache(1 << 20)
	load := fixed("page", Policy{TTL: time.Minute, Stale: time.Minute})
	do(t, cache, "a", load)
	tick.Advance(3 * time.Minute)
	if _, status := do(t, cache, "a", load); status != StatusMiss {
		t.Errorf("status = %v", status)
	}
	if entries := cache.Stats().Entries; entries != 1 {
		t.Errorf("entries = %d, want the reload stored", entries)
	}
}

func TestUncacheableResultsAreNotStored(t *testing.T) {
	cache, _ := newCache(1 << 20)
	load := fixed("page", Policy{})
	do(t, cache, "a", load)
	if cache.Stats().Entries != 0 {
		t.Errorf("stats = %+v", cache.Stats())
	}
	if _, status := do(t, cache, "a", load); status != StatusBypass {
		t.Errorf("status = %v, want a bypass", status)
	}
}

func TestTheOldestEntryIsEvictedFirst(t *testing.T) {
	cache, _ := newCache(2*entryOverhead + 8)
	do(t, cache, "a", fixed("aaaa", Policy{TTL: time.Minute}))
	do(t, cache, "b", fixed("bbbb", Policy{TTL: time.Minute}))
	do(t, cache, "a", fixed("aaaa", Policy{TTL: time.Minute}))
	do(t, cache, "c", fixed("cccc", Policy{TTL: time.Minute}))

	if _, status := do(t, cache, "b", fixed("bbbb", Policy{TTL: time.Minute})); status != StatusMiss {
		t.Errorf("the least recently used entry must go first, status = %v", status)
	}
	if cache.Stats().Evicted == 0 {
		t.Errorf("stats = %+v", cache.Stats())
	}
}

func TestAnEntryLargerThanTheLimitIsRefused(t *testing.T) {
	cache, _ := newCache(entryOverhead + 2)
	do(t, cache, "a", fixed("far too long", Policy{TTL: time.Minute}))
	if cache.Stats().Entries != 0 {
		t.Errorf("stats = %+v", cache.Stats())
	}
}

func TestStoringTheSameKeyTwiceReplacesIt(t *testing.T) {
	cache, _ := newCache(1 << 20)
	do(t, cache, "a", fixed("first", Policy{TTL: time.Minute}))
	cache.store("a", Value{Body: []byte("second")}, Policy{TTL: time.Minute})
	if body, _ := do(t, cache, "a", fixed("third", Policy{TTL: time.Minute})); body != "second" {
		t.Errorf("body = %q", body)
	}
	if entries := cache.Stats().Entries; entries != 1 {
		t.Errorf("entries = %d", entries)
	}
}

func TestTagsInvalidateEveryEntryThatCarriesThem(t *testing.T) {
	cache, _ := newCache(1 << 20)
	do(t, cache, "a", fixed("page a", Policy{TTL: time.Minute}, "listing:1", "nav"))
	do(t, cache, "b", fixed("page b", Policy{TTL: time.Minute}, "listing:1"))
	do(t, cache, "c", fixed("page c", Policy{TTL: time.Minute}, "listing:2"))

	if removed := cache.Invalidate("listing:1"); removed != 2 {
		t.Errorf("removed = %d", removed)
	}
	if _, status := do(t, cache, "c", fixed("page c", Policy{TTL: time.Minute})); status != StatusHit {
		t.Errorf("an untagged entry survives, status = %v", status)
	}
	if removed := cache.Invalidate("nothing"); removed != 0 {
		t.Errorf("removed = %d", removed)
	}
}

func TestPurgeEmptiesTheCache(t *testing.T) {
	cache, _ := newCache(1 << 20)
	do(t, cache, "a", fixed("page", Policy{TTL: time.Minute}, "tag"))
	cache.Purge()
	stats := cache.Stats()
	if stats.Entries != 0 || stats.Bytes != 0 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestALoaderFailureIsReportedAndNotStored(t *testing.T) {
	cache, _ := newCache(1 << 20)
	_, _, err := cache.Do("a", func(bool) (Value, Policy, error) {
		return Value{}, Policy{TTL: time.Minute}, errors.New("nope")
	})
	if err == nil {
		t.Fatal("expected the loader error")
	}
	if cache.Stats().Entries != 0 {
		t.Errorf("stats = %+v", cache.Stats())
	}
}

func TestAZeroLimitBypassesTheCache(t *testing.T) {
	cache := New(Options{})
	body, status, err := cache.Do("a", fixed("page", Policy{TTL: time.Minute}))
	if err != nil || string(body.Body) != "page" || status != StatusBypass {
		t.Errorf("body = %q, status = %v, err = %v", body.Body, status, err)
	}
	if cache.Stats().Entries != 0 {
		t.Errorf("stats = %+v", cache.Stats())
	}
}

func TestConcurrentMissesLoadOnce(t *testing.T) {
	cache, _ := newCache(1 << 20)
	var calls atomic.Int64
	release := make(chan struct{})
	load := func(bool) (Value, Policy, error) {
		calls.Add(1)
		<-release
		return Value{Body: []byte("page")}, Policy{TTL: time.Minute}, nil
	}
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, _, _ = cache.Do("a", load)
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	group.Wait()
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want one", calls.Load())
	}
}

func TestStatusNames(t *testing.T) {
	names := map[Status]string{
		StatusMiss:   "miss",
		StatusHit:    "hit",
		StatusStale:  "stale",
		StatusBypass: "bypass",
	}
	for status, want := range names {
		if got := status.String(); got != want {
			t.Errorf("%d = %q, want %q", status, got, want)
		}
	}
}

func TestStatsReportTheLimit(t *testing.T) {
	cache, _ := newCache(4096)
	do(t, cache, "a", fixed("page", Policy{TTL: time.Minute}))
	stats := cache.Stats()
	if stats.Limit != 4096 || stats.Bytes <= 0 || stats.Misses != 1 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestAPanickingRevalidationDoesNotTakeTheProcessDown(t *testing.T) {
	cache := New(Options{Limit: 1 << 20})
	loads := 0
	load := func(bool) (Value, Policy, error) {
		loads++
		if loads > 1 {
			panic("upstream exploded")
		}
		return Value{Body: []byte("first")}, Policy{TTL: time.Nanosecond, Stale: time.Hour}, nil
	}
	if _, _, err := cache.Do("k", load); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if _, _, err := cache.Do("k", load); err != nil {
		t.Fatalf("Do: %v", err)
	}
	cache.Wait()
}

func TestTheKeyCountsAgainstTheBudget(t *testing.T) {
	c := New(Options{Limit: 4 << 10})
	long := strings.Repeat("k", 512)
	c.Put(long, Value{Body: []byte("x")}, Policy{TTL: time.Minute})
	if used := c.Stats().Bytes; used < 1024 {
		t.Errorf("bytes = %d, want the key stored twice counted", used)
	}
}

func TestAnEndlessKeyIsNotCached(t *testing.T) {
	c := New(Options{Limit: 1 << 20})
	key := strings.Repeat("q", MaxKeyBytes+1)
	c.Put(key, Value{Body: []byte("x")}, Policy{TTL: time.Minute})
	if _, ok := c.Peek(key); ok {
		t.Error("a key past the cap must not enter the cache")
	}
	if used := c.Stats().Bytes; used != 0 {
		t.Errorf("bytes = %d, want nothing stored", used)
	}
}

func TestJunkQueriesCannotOutgrowTheBudget(t *testing.T) {
	const limit = 64 << 10
	c := New(Options{Limit: limit})
	for i := range 2000 {
		key := strconv.Itoa(i) + strings.Repeat("j", 900)
		c.Put(key, Value{Body: []byte("x")}, Policy{TTL: time.Minute})
	}
	if used := c.Stats().Bytes; used > limit {
		t.Errorf("bytes = %d, want the accounting to stay inside %d", used, limit)
	}
}

func TestTheForegroundFillIsNotToldItIsBackground(t *testing.T) {
	cache, _ := newCache(1 << 20)
	var seen []bool
	load := func(background bool) (Value, Policy, error) {
		seen = append(seen, background)
		return Value{Body: []byte("x")}, Policy{TTL: time.Minute}, nil
	}
	do(t, cache, "a", load)
	if len(seen) != 1 || seen[0] {
		t.Errorf("seen = %v, want one foreground fill", seen)
	}
}

func TestARefreshTellsTheLoaderItIsInTheBackground(t *testing.T) {
	cache, tick := newCache(1 << 20)
	var mu sync.Mutex
	var seen []bool
	load := func(background bool) (Value, Policy, error) {
		mu.Lock()
		seen = append(seen, background)
		mu.Unlock()
		return Value{Body: []byte("x")}, Policy{TTL: time.Minute, Stale: time.Hour}, nil
	}
	do(t, cache, "a", load)
	tick.Advance(2 * time.Minute)
	do(t, cache, "a", load)
	cache.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Fatalf("seen = %v, want a fill and a refresh", seen)
	}
	if seen[0] || !seen[1] {
		t.Errorf("seen = %v, want the refresh marked background", seen)
	}
}

func TestADisabledCacheLoadsInTheForeground(t *testing.T) {
	cache := New(Options{Limit: 0})
	var seen []bool
	_, _, err := cache.Do("a", func(background bool) (Value, Policy, error) {
		seen = append(seen, background)
		return Value{Body: []byte("x")}, Policy{TTL: time.Minute}, nil
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if len(seen) != 1 || seen[0] {
		t.Errorf("seen = %v, want one foreground load", seen)
	}
}

package cache

import (
	"container/list"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	entryOverhead = 96
	MaxKeyBytes   = 1 << 10
)

type Status uint8

const (
	StatusMiss Status = iota
	StatusHit
	StatusStale
	StatusBypass
)

func (s Status) String() string {
	switch s {
	case StatusHit:
		return "hit"
	case StatusStale:
		return "stale"
	case StatusBypass:
		return "bypass"
	default:
		return "miss"
	}
}

type Value struct {
	Body   []byte
	Tags   []string
	Policy Policy
}

func (v Value) weight() int64 {
	total := int64(len(v.Body)) + entryOverhead
	for _, tag := range v.Tags {
		total += int64(len(tag))
	}
	return total
}

func weigh(key string, value Value) int64 {
	return value.weight() + int64(2*len(key))
}

type Policy struct {
	TTL   time.Duration
	Stale time.Duration
}

func (p Policy) cacheable() bool {
	return p.TTL > 0
}

type Loader func(background bool) (Value, Policy, error)

type Options struct {
	Limit int64
	Clock func() time.Time
}

type entry struct {
	key     string
	value   Value
	fresh   time.Time
	expires time.Time
	weight  int64
}

type Cache struct {
	mu       sync.Mutex
	limit    int64
	used     int64
	items    map[string]*list.Element
	order    *list.List
	tags     map[string]map[string]struct{}
	clock    func() time.Time
	group    singleflight.Group
	refresh  sync.WaitGroup
	hits     int64
	misses   int64
	stales   int64
	evicted  int64
	disabled bool
}

func New(opts Options) *Cache {
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Cache{
		limit:    opts.Limit,
		items:    map[string]*list.Element{},
		order:    list.New(),
		tags:     map[string]map[string]struct{}{},
		clock:    clock,
		disabled: opts.Limit <= 0,
	}
}

type Stats struct {
	Entries int
	Bytes   int64
	Limit   int64
	Hits    int64
	Misses  int64
	Stales  int64
	Evicted int64
}

func (c *Cache) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Stats{
		Entries: len(c.items),
		Bytes:   c.used,
		Limit:   c.limit,
		Hits:    c.hits,
		Misses:  c.misses,
		Stales:  c.stales,
		Evicted: c.evicted,
	}
}

func (c *Cache) Do(key string, load Loader) (Value, Status, error) {
	if c.disabled {
		value, policy, err := load(false)
		value.Policy = policy
		return value, StatusBypass, err
	}
	if value, status, ok := c.peek(key); ok {
		if status == StatusStale {
			c.revalidate(key, load)
		}
		return value, status, nil
	}
	return c.fill(key, load, false)
}

func (c *Cache) Peek(key string) ([]byte, bool) {
	if c.disabled {
		return nil, false
	}
	value, status, ok := c.peek(key)
	if !ok || status == StatusStale {
		return nil, false
	}
	return value.Body, true
}

func (c *Cache) Put(key string, value Value, policy Policy) {
	if c.disabled || !policy.cacheable() {
		return
	}
	c.store(key, value, policy)
}

func (c *Cache) peek(key string) (Value, Status, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.items[key]
	if !ok {
		c.misses++
		return Value{}, StatusMiss, false
	}
	held := element.Value.(*entry)
	now := c.clock()
	if now.After(held.expires) {
		c.drop(element)
		c.misses++
		return Value{}, StatusMiss, false
	}
	c.order.MoveToFront(element)
	if now.After(held.fresh) {
		c.stales++
		return held.value, StatusStale, true
	}
	c.hits++
	return held.value, StatusHit, true
}

type filled struct {
	value  Value
	status Status
}

func (c *Cache) fill(key string, load Loader, background bool) (Value, Status, error) {
	result, err, _ := c.group.Do(key, func() (any, error) {
		value, policy, err := load(background)
		if err != nil {
			return filled{}, err
		}
		value.Policy = policy
		if !policy.cacheable() {
			return filled{value: value, status: StatusBypass}, nil
		}
		c.store(key, value, policy)
		return filled{value: value, status: StatusMiss}, nil
	})
	if err != nil {
		return Value{}, StatusMiss, err
	}
	outcome := result.(filled)
	return outcome.value, outcome.status, nil
}

func (c *Cache) revalidate(key string, load Loader) {
	c.refresh.Add(1)
	go func() {
		defer c.refresh.Done()
		defer func() { _ = recover() }()
		_, _, _ = c.fill(key, load, true)
	}()
}

func (c *Cache) Wait() {
	c.refresh.Wait()
}

func (c *Cache) store(key string, value Value, policy Policy) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.items[key]; ok {
		c.drop(existing)
	}
	if len(key) > MaxKeyBytes {
		return
	}
	now := c.clock()
	value.Policy = policy
	held := &entry{
		key:     key,
		value:   value,
		fresh:   now.Add(policy.TTL),
		expires: now.Add(policy.TTL + policy.Stale),
		weight:  weigh(key, value),
	}
	if held.weight > c.limit {
		return
	}
	element := c.order.PushFront(held)
	c.items[key] = element
	c.used += held.weight
	for _, tag := range value.Tags {
		keys, ok := c.tags[tag]
		if !ok {
			keys = map[string]struct{}{}
			c.tags[tag] = keys
		}
		keys[key] = struct{}{}
	}
	for c.used > c.limit {
		c.drop(c.order.Back())
		c.evicted++
	}
}

func (c *Cache) drop(element *list.Element) {
	held := element.Value.(*entry)
	c.order.Remove(element)
	delete(c.items, held.key)
	c.used -= held.weight
	for _, tag := range held.value.Tags {
		if keys, ok := c.tags[tag]; ok {
			delete(keys, held.key)
			if len(keys) == 0 {
				delete(c.tags, tag)
			}
		}
	}
}

func (c *Cache) Invalidate(tags ...string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	removed := 0
	for _, tag := range tags {
		for key := range c.tags[tag] {
			if element, ok := c.items[key]; ok {
				c.drop(element)
				removed++
			}
		}
	}
	return removed
}

func (c *Cache) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = map[string]*list.Element{}
	c.order.Init()
	c.tags = map[string]map[string]struct{}{}
	c.used = 0
}

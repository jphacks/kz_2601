package extapi

import (
	"slices"
	"strings"
	"sync"
	"time"
)

// The answer cache (docs/decisions/external-apis.md「キャッシュ」): a read op
// of a provider without OAuth (OpSpec.CacheTTL > 0) keeps the answer of the
// same arguments in this instance's memory for as long as the provider's
// terms allow. A call answered from it counts no `runs` and no coins.
// OAuth ops (a person's own data) and write ops are never cached.

// maxCacheEntries bounds the cache of one instance.
const maxCacheEntries = 2048

type cacheEntry struct {
	out     Output
	expires time.Time
}

type callCache struct {
	mu sync.Mutex
	m  map[string]cacheEntry
}

func newCallCache() *callCache { return &callCache{m: map[string]cacheEntry{}} }

// cacheKey is provider, op and the arguments in name order.
func cacheKey(provider, op string, args map[string]string) string {
	names := make([]string, 0, len(args))
	for k := range args {
		names = append(names, k)
	}
	slices.Sort(names)
	var b strings.Builder
	b.WriteString(provider + "\x00" + op)
	for _, k := range names {
		b.WriteString("\x00" + k + "=" + args[k])
	}
	return b.String()
}

func (c *callCache) get(key string, now time.Time) (Output, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok {
		return Output{}, false
	}
	if !now.Before(e.expires) {
		delete(c.m, key)
		return Output{}, false
	}
	return e.out.clone(), true
}

func (c *callCache) put(key string, out Output, expires time.Time, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= maxCacheEntries {
		for k, e := range c.m {
			if !now.Before(e.expires) {
				delete(c.m, k)
			}
		}
		if len(c.m) >= maxCacheEntries {
			c.m = map[string]cacheEntry{}
		}
	}
	out.Usage = Usage{}
	c.m[key] = cacheEntry{out: out.clone(), expires: expires}
}

// clone copies the rows so a cached answer is never shared.
func (o Output) clone() Output {
	o.Rows = slices.Clone(o.Rows)
	if o.Item != nil {
		it := *o.Item
		o.Item = &it
	}
	return o
}

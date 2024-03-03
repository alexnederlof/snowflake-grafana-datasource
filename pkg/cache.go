package main

import (
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/hashicorp/golang-lru/v2/expirable"
)

type CacheKey struct {
	Query           string
	MaxDataPoints   int64
	DurationSeconds int64
}

type CacheValue struct {
	Tables    []DataTable
	TimeRange backend.TimeRange
}

/**
* Checks if the cached data is still within the allowed time range.
 */
func (ck *CacheKey) Expired(cached backend.TimeRange, incoming backend.TimeRange, maxTtlMinutes int) bool {
	secondsPerBucket := ck.DurationSeconds / ck.MaxDataPoints
	if secondsPerBucket > int64(maxTtlMinutes)*60 {
		secondsPerBucket = int64(maxTtlMinutes) * 60
	}
	log.DefaultLogger.Debug("Cache compare ",
		"it", incoming.To,
		"ct", cached.To,
		"if", incoming.From,
		"cf", cached.From,
		"secondsPerBucket", secondsPerBucket)
	return int64(cached.To.Sub(incoming.To).Abs().Seconds()) > secondsPerBucket || int64(cached.From.Sub(incoming.From).Abs().Seconds()) > secondsPerBucket

}

func NewCache(maxEntries int, ttlMinutes int) *expirable.LRU[CacheKey, CacheValue] {
	c := expirable.NewLRU[CacheKey, CacheValue](
		maxEntries,
		nil,
		time.Minute*time.Duration(ttlMinutes),
	)
	return c
}

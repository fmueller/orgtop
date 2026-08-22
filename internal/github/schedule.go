package github

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// defaultInterval is the v0.1.0 polling floor (FR-004). No successful poll delay
// and no failure retry delay is ever shorter than this.
const defaultInterval = 60 * time.Second

// Scheduling headers OrgTop reads. Values that do not parse are ignored safely.
const (
	headerPollInterval       = "X-Poll-Interval"
	headerRetryAfter         = "Retry-After"
	headerRateLimitRemaining = "X-RateLimit-Remaining"
	headerRateLimitReset     = "X-RateLimit-Reset"
)

// pollDelay returns the interval before the next refresh may start: the greatest
// of the default interval and every advertised X-Poll-Interval of one refresh.
func pollDelay(advertised []time.Duration) time.Duration {
	delay := defaultInterval
	for _, interval := range advertised {
		delay = max(delay, interval)
	}
	return delay
}

// pollInterval returns the X-Poll-Interval seconds of one response, or zero when
// the header is absent or unusable.
func pollInterval(header http.Header) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(header.Get(headerPollInterval)))
	if err != nil || seconds < 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

// retryDelay returns the delay before the next attempt for the given repository
// is eligible. Rate-limited responses consider both Retry-After and
// X-RateLimit-Reset, and the latest resulting retry instant wins.
func retryDelay(header http.Header, rateLimited bool, now time.Time) time.Duration {
	if !rateLimited {
		return defaultInterval
	}
	delay := defaultInterval
	if after, valid := retryAfterDelay(header, now); valid {
		delay = max(delay, after)
	}
	if reset, valid := rateLimitResetDelay(header, now); valid {
		delay = max(delay, reset)
	}
	return delay
}

// isRateLimited reports whether a response denies the request because the rate
// limit is exhausted. 429 always is; 403 only when X-RateLimit-Remaining parses
// as zero or a valid Retry-After is present (FR-003).
func isRateLimited(status int, header http.Header, now time.Time) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	if status != http.StatusForbidden {
		return false
	}
	if remaining, err := strconv.Atoi(strings.TrimSpace(header.Get(headerRateLimitRemaining))); err == nil && remaining == 0 {
		return true
	}
	_, valid := retryAfterDelay(header, now)
	return valid
}

// retryAfterDelay parses Retry-After as delta-seconds or an HTTP date.
func retryAfterDelay(header http.Header, now time.Time) (time.Duration, bool) {
	value := strings.TrimSpace(header.Get(headerRetryAfter))
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	if instant, err := http.ParseTime(value); err == nil {
		return delayUntil(instant, now), true
	}
	return 0, false
}

// rateLimitResetDelay parses X-RateLimit-Reset as a Unix timestamp.
func rateLimitResetDelay(header http.Header, now time.Time) (time.Duration, bool) {
	value := strings.TrimSpace(header.Get(headerRateLimitReset))
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds < 0 {
		return 0, false
	}
	return delayUntil(time.Unix(seconds, 0), now), true
}

// delayUntil returns the non-negative distance from now to instant.
func delayUntil(instant, now time.Time) time.Duration {
	return max(instant.Sub(now), 0)
}

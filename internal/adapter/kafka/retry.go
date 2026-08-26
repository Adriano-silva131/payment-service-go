package kafka

import (
	"strconv"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Retry topic/header design (see payment-service-go README for the full rationale):
// one generic "<topic>-retry" topic instead of Spring Kafka's N suffixed retry topics,
// same 4-attempts/2s->4s->8s(cap 10s) backoff math as the old @RetryableTopic config.
const (
	MaxAttempts = 4
	baseDelay   = 2 * time.Second
	maxDelay    = 10 * time.Second

	HeaderEventType      = "event-type"
	HeaderRetryAttempt   = "x-retry-attempt"
	HeaderRetryNotBefore = "x-retry-not-before-ms"
	HeaderOriginalTopic  = "x-original-topic"
	HeaderOriginalKey    = "x-original-key"
)

// RetryTopic returns the single retry topic name for a given main topic.
func RetryTopic(mainTopic string) string {
	return mainTopic + "-retry"
}

// NextBackoff returns the delay before attempt number `attempt` (1-based) should run,
// mirroring @BackOff(delay=2000, multiplier=2.0, maxDelay=10000).
func NextBackoff(attempt int) time.Duration {
	delay := baseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay > maxDelay {
			return maxDelay
		}
	}
	return delay
}

func headerValue(headers []kgo.RecordHeader, key string) (string, bool) {
	for _, h := range headers {
		if h.Key == key {
			return string(h.Value), true
		}
	}
	return "", false
}

func AttemptFromHeaders(headers []kgo.RecordHeader) int {
	v, ok := headerValue(headers, HeaderRetryAttempt)
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

func EventTypeFromHeaders(headers []kgo.RecordHeader) string {
	v, _ := headerValue(headers, HeaderEventType)
	return v
}

func NotBeforeFromHeaders(headers []kgo.RecordHeader) time.Time {
	v, ok := headerValue(headers, HeaderRetryNotBefore)
	if !ok {
		return time.Time{}
	}
	ms, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// BuildRetryHeaders constructs the header set for republishing a failed record onto the
// retry topic for the given (1-based) attempt number.
func BuildRetryHeaders(originalTopic, originalKey, eventType string, attempt int) []kgo.RecordHeader {
	notBefore := time.Now().Add(NextBackoff(attempt))
	return []kgo.RecordHeader{
		{Key: HeaderEventType, Value: []byte(eventType)},
		{Key: HeaderRetryAttempt, Value: []byte(strconv.Itoa(attempt))},
		{Key: HeaderRetryNotBefore, Value: []byte(strconv.FormatInt(notBefore.UnixMilli(), 10))},
		{Key: HeaderOriginalTopic, Value: []byte(originalTopic)},
		{Key: HeaderOriginalKey, Value: []byte(originalKey)},
	}
}

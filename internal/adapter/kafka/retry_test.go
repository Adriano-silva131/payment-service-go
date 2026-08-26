package kafka

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/twmb/franz-go/pkg/kgo"
)

func TestNextBackoff_MatchesJavaBackOffConfig(t *testing.T) {
	// @BackOff(delay=2000, multiplier=2.0, maxDelay=10000): 2s, 4s, 8s, capped at 10s.
	assert.Equal(t, 2*time.Second, NextBackoff(1))
	assert.Equal(t, 4*time.Second, NextBackoff(2))
	assert.Equal(t, 8*time.Second, NextBackoff(3))
	assert.Equal(t, 10*time.Second, NextBackoff(4))
	assert.Equal(t, 10*time.Second, NextBackoff(5))
}

func TestRetryTopic(t *testing.T) {
	assert.Equal(t, "order-events-retry", RetryTopic("order-events"))
}

func TestBuildRetryHeaders_RoundTrip(t *testing.T) {
	headers := BuildRetryHeaders("order-events", "order-123", "order.created.v1", 2)

	assert.Equal(t, "order.created.v1", EventTypeFromHeaders(headers))
	assert.Equal(t, 2, AttemptFromHeaders(headers))
	notBefore := NotBeforeFromHeaders(headers)
	assert.WithinDuration(t, time.Now().Add(4*time.Second), notBefore, 1*time.Second)
}

func TestAttemptFromHeaders_DefaultsToZeroWhenMissing(t *testing.T) {
	assert.Equal(t, 0, AttemptFromHeaders([]kgo.RecordHeader{}))
}

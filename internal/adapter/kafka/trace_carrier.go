package kafka

import "github.com/twmb/franz-go/pkg/kgo"

type headerCarrier struct {
	headers *[]kgo.RecordHeader
}

func newHeaderCarrier(headers *[]kgo.RecordHeader) headerCarrier {
	return headerCarrier{headers: headers}
}

func (c headerCarrier) Get(key string) string {
	v, _ := headerValue(*c.headers, key)
	return v
}

func (c headerCarrier) Set(key, value string) {
	*c.headers = append(*c.headers, kgo.RecordHeader{Key: key, Value: []byte(value)})
}

func (c headerCarrier) Keys() []string {
	keys := make([]string, len(*c.headers))
	for i, h := range *c.headers {
		keys[i] = h.Key
	}
	return keys
}

var traceContextHeaderKeys = []string{"traceparent", "tracestate"}

func traceHeadersFrom(headers []kgo.RecordHeader) []kgo.RecordHeader {
	var out []kgo.RecordHeader
	for _, key := range traceContextHeaderKeys {
		if v, ok := headerValue(headers, key); ok {
			out = append(out, kgo.RecordHeader{Key: key, Value: []byte(v)})
		}
	}
	return out
}

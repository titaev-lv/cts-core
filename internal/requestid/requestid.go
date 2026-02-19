package requestid

import "context"

type contextKey string

const requestIDKey contextKey = "request_id"

// WithContext stores request_id in context when present.
func WithContext(ctx context.Context, requestID string) context.Context {
	if ctx == nil || requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, requestID)
}

// FromContext returns request_id from context if set.
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value := ctx.Value(requestIDKey)
	requestID, _ := value.(string)
	return requestID
}

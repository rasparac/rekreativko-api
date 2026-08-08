package api

import "context"

type contextKey string

const (
	requestIDKey contextKey = "request_id"
	ipAddressKey contextKey = "ip_address"
	userAgentKey contextKey = "user_agent"
)

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	if v := ctx.Value(requestIDKey); v != nil {
		if requestID, ok := v.(string); ok {
			return requestID
		}
	}
	return ""
}

func WithIpAddress(ctx context.Context, ipAddress string) context.Context {
	return context.WithValue(ctx, ipAddressKey, ipAddress)
}

func IpAddressFromContext(ctx context.Context) string {
	if v := ctx.Value(ipAddressKey); v != nil {
		if ipAddress, ok := v.(string); ok {
			return ipAddress
		}
	}
	return ""
}

func WithUserAgent(ctx context.Context, userAgent string) context.Context {
	return context.WithValue(ctx, userAgentKey, userAgent)
}

func UserAgentFromContext(ctx context.Context) string {
	if v := ctx.Value(userAgentKey); v != nil {
		if userAgent, ok := v.(string); ok {
			return userAgent
		}
	}
	return ""
}

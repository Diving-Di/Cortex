package ai

import "context"

type RequestMetadata struct {
	RequestID   string `json:"request_id"`
	RequestType string `json:"request_type"`
	Tenant      string `json:"tenant"`
	Environment string `json:"environment"`
}

type metadataContextKey struct{}

func WithRequestMetadata(ctx context.Context, metadata RequestMetadata) context.Context {
	return context.WithValue(ctx, metadataContextKey{}, metadata)
}

func requestMetadataFrom(ctx context.Context) RequestMetadata {
	metadata, _ := ctx.Value(metadataContextKey{}).(RequestMetadata)
	return metadata
}

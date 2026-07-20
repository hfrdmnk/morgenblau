package api

import (
	"context"

	"morgenblau/internal/sharemeta"
)

type ShareMetadataResolver interface {
	ResolveMany(ctx context.Context, targets []sharemeta.Target) []sharemeta.Metadata
}

func resolveShareMetadata(ctx context.Context, resolver ShareMetadataResolver, targets []sharemeta.Target) []sharemeta.Metadata {
	out := make([]sharemeta.Metadata, len(targets))
	if resolver == nil || len(targets) == 0 {
		return out
	}
	resolved := resolver.ResolveMany(ctx, targets)
	copy(out, resolved)
	return out
}

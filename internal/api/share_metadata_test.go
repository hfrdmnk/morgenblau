package api

import (
	"context"

	"morgenblau/internal/sharemeta"
)

type fakeShareMetadataResolver struct {
	byKey map[string]sharemeta.Metadata
}

func noShareMetadata() *fakeShareMetadataResolver {
	return &fakeShareMetadataResolver{byKey: map[string]sharemeta.Metadata{}}
}

func (f *fakeShareMetadataResolver) ResolveMany(_ context.Context, targets []sharemeta.Target) []sharemeta.Metadata {
	out := make([]sharemeta.Metadata, len(targets))
	for i, target := range targets {
		key := target.Document
		if key == "" {
			key = target.ItemURL
		}
		out[i] = f.byKey[key]
	}
	return out
}

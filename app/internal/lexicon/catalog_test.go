package lexicon

import "testing"

func TestValidateRecord(t *testing.T) {
	cases := []struct {
		name    string
		nsid    string
		record  map[string]any
		wantErr bool
	}{
		{
			name: "follow valid",
			nsid: Follow,
			record: map[string]any{
				"subject":   "did:plc:abc123",
				"createdAt": "2026-07-09T00:00:00Z",
			},
		},
		{
			name: "follow missing required subject",
			nsid: Follow,
			record: map[string]any{
				"createdAt": "2026-07-09T00:00:00Z",
			},
			wantErr: true,
		},
		{
			name: "follow subject wrong type",
			nsid: Follow,
			record: map[string]any{
				"subject":   123,
				"createdAt": "2026-07-09T00:00:00Z",
			},
			wantErr: true,
		},
		{
			name: "subscription valid rssFeed variant",
			nsid: Subscription,
			record: map[string]any{
				"source": map[string]any{
					"$type":   SourceRSS,
					"feedUrl": "https://example.test/feed.xml",
				},
				"createdAt": "2026-07-09T00:00:00Z",
			},
		},
		{
			name: "subscription valid standardPublication variant",
			nsid: Subscription,
			record: map[string]any{
				"source": map[string]any{
					"$type":       SourceStandard,
					"publication": "at://did:plc:pub/site.standard.publication/3pub",
				},
				"createdAt": "2026-07-09T00:00:00Z",
			},
		},
		{
			name: "subscription missing required source",
			nsid: Subscription,
			record: map[string]any{
				"createdAt": "2026-07-09T00:00:00Z",
			},
			wantErr: true,
		},
		{
			name: "subscription source variant missing required feedUrl",
			nsid: Subscription,
			record: map[string]any{
				"source": map[string]any{
					"$type": SourceRSS,
				},
				"createdAt": "2026-07-09T00:00:00Z",
			},
			wantErr: true,
		},
		{
			name: "share valid",
			nsid: Share,
			record: map[string]any{
				"itemUrl":   "https://example.test/post",
				"createdAt": "2026-07-09T00:00:00Z",
			},
		},
		{
			name: "share missing required itemUrl",
			nsid: Share,
			record: map[string]any{
				"createdAt": "2026-07-09T00:00:00Z",
			},
			wantErr: true,
		},
		{
			name: "share comment wrong type",
			nsid: Share,
			record: map[string]any{
				"itemUrl":   "https://example.test/post",
				"createdAt": "2026-07-09T00:00:00Z",
				"comment":   42,
			},
			wantErr: true,
		},
		{
			name: "save valid",
			nsid: Save,
			record: map[string]any{
				"itemUrl":   "https://example.test/post",
				"createdAt": "2026-07-09T00:00:00Z",
			},
		},
		{
			name: "save missing required itemUrl",
			nsid: Save,
			record: map[string]any{
				"createdAt": "2026-07-09T00:00:00Z",
			},
			wantErr: true,
		},
		{
			name: "save valid with native []string tags",
			nsid: Save,
			record: map[string]any{
				"itemUrl":   "https://example.test/post",
				"createdAt": "2026-07-09T00:00:00Z",
				"tags":      []string{"News", "Tech"},
			},
		},
		{
			name: "unknown nsid errors",
			nsid: "blue.morgen.does.not.exist",
			record: map[string]any{
				"createdAt": "2026-07-09T00:00:00Z",
			},
			wantErr: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRecord(tt.nsid, tt.record)
			if tt.wantErr && err == nil {
				t.Fatalf("ValidateRecord(%s, %v) = nil, want error", tt.nsid, tt.record)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ValidateRecord(%s, %v) = %v, want nil", tt.nsid, tt.record, err)
			}
		})
	}
}

func TestValidateRecordLenient_AllowsRelaxedDatetime(t *testing.T) {
	record := map[string]any{
		"subject":   "did:plc:abc123",
		"createdAt": "2026-07-09T00:00:00-0000", // legacy offset syntax, not strict RFC-3339
	}
	if err := ValidateRecord(Follow, record); err == nil {
		t.Fatalf("ValidateRecord accepted a relaxed datetime, want strict rejection")
	}
	if err := ValidateRecordLenient(Follow, record); err != nil {
		t.Errorf("ValidateRecordLenient rejected a relaxed-but-legal datetime: %v", err)
	}
}

func TestValidateRecord_DoesNotMutateCallerMap(t *testing.T) {
	record := map[string]any{
		"itemUrl":   "https://example.test/post",
		"createdAt": "2026-07-09T00:00:00Z",
	}
	if err := ValidateRecord(Save, record); err != nil {
		t.Fatalf("ValidateRecord: %v", err)
	}
	if _, ok := record["$type"]; ok {
		t.Errorf("ValidateRecord mutated caller's record map by adding $type: %v", record)
	}
}

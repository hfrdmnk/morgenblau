package newsletter

import (
	"errors"
	"time"
)

var (
	ErrInvalid      = errors.New("invalid newsletter request")
	ErrNotFound     = errors.New("newsletter item not found")
	ErrStorageQuota = errors.New("newsletter storage quota reached")
	ErrUnavailable  = errors.New("newsletter ingestion is unavailable")
)

type Config struct {
	Domain             string
	OwnerStorageBytes  int64
	GlobalStorageBytes int64
}

type SourceStatus string

const (
	SourceActive  SourceStatus = "active"
	SourceStopped SourceStatus = "stopped"
)

type Source struct {
	ID              string
	Title           string
	SenderName      *string
	SenderAddress   string
	Status          SourceStatus
	Primary         bool
	Tags            []string
	FirstReceivedAt *time.Time
	LastReceivedAt  *time.Time
	IssueCount      int64
	SavedCount      int64
	Count7d         int64
	Count28d        int64
	Count56d        int64
	Count84d        int64
}

type SourceGroups struct {
	Active  []Source
	Stopped []Source
}

type SourcePatch struct {
	Title   string
	Primary bool
	Tags    []string
}

type Message struct {
	ID                     string
	EntrySlug              string
	SourceID               string
	SourceTitle            string
	SourcePrimary          bool
	Title                  string
	SenderName             *string
	SenderAddress          string
	SentAt                 *time.Time
	ReceivedAt             time.Time
	BodyHTML               string
	BodyText               string
	HasBlockedRemoteImages bool
	RemoteImagesAllowed    bool
	SaveID                 *string
	SavedAt                *time.Time
}

type MoveTarget struct {
	SourceID       string
	NewSourceTitle string
}

type Save struct {
	ID        string
	MessageID string
	CreatedAt time.Time
}

type SaveItem struct {
	ID          string
	MessageID   string
	CreatedAt   time.Time
	Title       string
	EntrySlug   string
	ReceivedAt  time.Time
	SourceTitle string
}

type InlineAsset struct {
	MediaType string
	Data      []byte
	ETag      string
}

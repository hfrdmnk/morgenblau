package websource

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"

	"morgenblau/internal/database/db"
	"morgenblau/internal/postkey"
	"morgenblau/internal/webfeed"
)

const sectionChecksPerSweep = 3

func needsSectionCheck(src db.WebSource) bool {
	var r webfeed.Recipe
	if src.AnchoredByDid != nil || json.Unmarshal([]byte(src.Recipe), &r) != nil {
		return false
	}
	return (r.Mode == webfeed.ModeStreams || r.Mode == webfeed.ModeCSS) && r.Setup.Anchor == "" && !r.Setup.SectionChecked
}

func (s *Service) checkSection(ctx context.Context, src db.WebSource) error {
	listing, err := parseHTTPURL(src.ListingUrl)
	if err != nil {
		return err
	}
	select {
	case s.slots <- struct{}{}:
	default:
		return ErrBusy
	}
	gen, err := func() (Generated, error) {
		defer func() { <-s.slots }()
		setupCtx, cancel := context.WithTimeout(ctx, s.setupTimeout)
		defer cancel()
		return s.setupInSlot(setupCtx, listing.String(), "", false)
	}()
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case err != nil:
		slog.Info("web source section check failed; not retried", "id", src.ID, "err", err)
	default:
		if moved, err := s.moveToSection(ctx, src, listing, gen); moved || err != nil {
			return err
		}
	}
	_, err = db.New(s.writer).MarkWebSourceSectionChecked(ctx, db.MarkWebSourceSectionCheckedParams{
		ID: src.ID, SetupSeq: src.SetupSeq, UpdatedAt: formatTime(s.now().UTC()),
	})
	return err
}

func (s *Service) moveToSection(ctx context.Context, src db.WebSource, listing *url.URL, gen Generated) (bool, error) {
	setup, err := newSetupRow(listingKey(listing), listing, gen)
	if err != nil || setup.key == setup.pastedKey || gen.Health != HealthOK {
		return false, err
	}
	current, err := s.currentPosts(ctx, src.ID)
	if err != nil {
		return false, err
	}
	have := map[string]bool{}
	for _, it := range gen.Items {
		have[postkey.URL(it.URL)] = true
	}
	for _, u := range current {
		if !have[postkey.URL(u)] {
			return false, nil
		}
	}
	slog.Info("web source moves to its section page", "id", src.ID, "from", src.ListingUrl, "to", setup.listingURL)
	setup.replaces, setup.atSeq = src.ID, &src.SetupSeq
	_, _, err = s.storeSetup(ctx, src.CreatedByDid, setup)
	return true, err
}

func (s *Service) currentPosts(ctx context.Context, id string) ([]string, error) {
	rows, err := s.read.ListWebSourcePolls(ctx, db.ListWebSourcePollsParams{SourceID: id, Limit: historyPolls})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		var urls []string
		if err := json.Unmarshal([]byte(r.Urls), &urls); err != nil {
			return nil, err
		}
		if len(urls) > 0 {
			return urls, nil
		}
	}
	return nil, nil
}

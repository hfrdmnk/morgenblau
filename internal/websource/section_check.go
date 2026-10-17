package websource

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"

	"morgenblau/internal/database/db"
	"morgenblau/internal/postkey"
	"morgenblau/internal/webfeed"
)

// The one-time section check moves sources stored before setup looked for a section page; once every recipe carries
// section_checked, this file, its PollDue hook and MarkWebSourceSectionChecked can go.

// sectionChecksPerSweep spreads the check of older sources over sweeps, so a deploy does not stampede sites.
const sectionChecksPerSweep = 3

// needsSectionCheck is a plain streams or css setup stored before setup looked for a section page.
func needsSectionCheck(src db.WebSource) bool {
	var r webfeed.Recipe
	if src.AnchoredByDid != nil || json.Unmarshal([]byte(src.Recipe), &r) != nil {
		return false
	}
	return (r.Mode == webfeed.ModeStreams || r.Mode == webfeed.ModeCSS) && r.Setup.Anchor == "" && !r.Setup.SectionChecked
}

// checkSection runs one free setup of a source's listing and moves the source when it found a healthy section page that
// keeps every post readers see now. Any other answer marks the recipe checked; only busy slots leave it for a later sweep.
func (s *Service) checkSection(ctx context.Context, src db.WebSource) error {
	listing, err := parseHTTPURL(src.ListingUrl)
	if err != nil {
		return err
	}
	setupCtx, cancel := context.WithTimeout(ctx, s.setupTimeout)
	defer cancel()
	gen, err := s.runSetup(setupCtx, listing.String(), "", false)
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.Is(err, ErrBusy):
		return err
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

// moveToSection stores gen as the source's setup when it moved to a healthy section page holding the source's current
// posts. Without the model the free setup may read the page differently than the setup in force, so readers must keep
// the list they followed.
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

// currentPosts are the URLs of the source's newest good poll that found any.
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

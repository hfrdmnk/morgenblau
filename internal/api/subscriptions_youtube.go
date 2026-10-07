package api

import (
	"encoding/csv"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var youtubeImportChannelID = regexp.MustCompile(`^UC[A-Za-z0-9_-]{22}$`)

func parseYouTubeCSV(input string) (importPlan, error) {
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(input, "\ufeff")))
	r.FieldsPerRecord = 3
	// Takeout localizes the header names, but keeps the column order.
	if _, err := r.Read(); err != nil {
		return importPlan{}, fmt.Errorf("Choose the YouTube subscriptions CSV file from Google Takeout")
	}
	var sources []importSource
	var warnings []string
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return importPlan{}, fmt.Errorf("Could not read the YouTube subscriptions CSV file")
		}
		channelID := strings.TrimSpace(row[0])
		if !youtubeImportChannelID.MatchString(channelID) {
			line, _ := r.FieldPos(0)
			warnings = append(warnings, fmt.Sprintf("Row %d: invalid YouTube channel ID", line))
			continue
		}
		sources = append(sources, importSource{
			FeedURL: "https://www.youtube.com/feeds/videos.xml?channel_id=" + channelID,
			SiteURL: "https://www.youtube.com/channel/" + channelID,
			Title:   row[2],
		})
	}
	plan := prepareImportSources(sources)
	plan.Warnings = append(plan.Warnings, warnings...)
	return plan, nil
}

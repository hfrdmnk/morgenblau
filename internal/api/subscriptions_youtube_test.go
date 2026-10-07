package api

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestImportPrepareYouTubeCSV(t *testing.T) {
	pds := &fakePDS{}
	input := "\ufeffKanal-ID,Kanal-URL,Kanaltitel\r\n" +
		"UCaaaaaaaaaaaaaaaaaaaaaa,http://www.youtube.com/channel/UCaaaaaaaaaaaaaaaaaaaaaa,\"Example, videos\"\r\n" +
		"UCbbbbbbbbbbbbbbbbbbbbbb,https://other.example.com/not-youtube,\"Example \"\"Two\"\"\"\r\n" +
		"UCaaaaaaaaaaaaaaaaaaaaaa,http://www.youtube.com/channel/UCaaaaaaaaaaaaaaaaaaaaaa,Duplicate\r\n" +
		"bad-id,https://www.youtube.com/channel/bad-id,Invalid channel\r\n"
	w := importRequest(t, SubscriptionsImportPrepareHandler(pds), map[string]string{"provider": "youtube", "csv": input})
	if w.Code != 200 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	var got importPlan
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := []importSource{
		{FeedURL: "https://www.youtube.com/feeds/videos.xml?channel_id=UCaaaaaaaaaaaaaaaaaaaaaa", SiteURL: "https://www.youtube.com/channel/UCaaaaaaaaaaaaaaaaaaaaaa", Title: "Example, videos"},
		{FeedURL: "https://www.youtube.com/feeds/videos.xml?channel_id=UCbbbbbbbbbbbbbbbbbbbbbb", SiteURL: "https://www.youtube.com/channel/UCbbbbbbbbbbbbbbbbbbbbbb", Title: `Example "Two"`},
	}
	if !reflect.DeepEqual(got.Sources, want) || !reflect.DeepEqual(got.Warnings, []string{"Row 5: invalid YouTube channel ID"}) {
		t.Fatalf("got %#v, want sources %#v and one warning", got, want)
	}
	if pds.listCalls != 0 || pds.creates != 0 || pds.puts != 0 {
		t.Fatal("preparing a CSV touched the PDS")
	}
}

func TestImportPrepareYouTubeRejectsMalformedCSV(t *testing.T) {
	for _, input := range []string{
		"",
		"not a csv",
		"Channel Id,Channel Url,Channel Title\nUCaaaaaaaaaaaaaaaaaaaaaa,missing-title\n",
		"Channel Id,Channel Url,Channel Title\nUCaaaaaaaaaaaaaaaaaaaaaa,url,\"unclosed\n",
	} {
		t.Run(input, func(t *testing.T) {
			w := importRequest(t, SubscriptionsImportPrepareHandler(&fakePDS{}), map[string]string{"provider": "youtube", "csv": input})
			if w.Code != 400 {
				t.Fatalf("%d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestImportPrepareYouTubeEmptyExport(t *testing.T) {
	w := importRequest(t, SubscriptionsImportPrepareHandler(&fakePDS{}), map[string]string{"provider": "youtube", "csv": "Channel Id,Channel Url,Channel Title\n"})
	if w.Code != 200 {
		t.Fatalf("%d: %s", w.Code, w.Body.String())
	}
	var got importPlan
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 0 || len(got.Warnings) != 0 {
		t.Fatalf("got %#v", got)
	}
}

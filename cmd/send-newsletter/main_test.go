package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseOptionsRequiresRecipient(t *testing.T) {
	_, err := parseOptions([]string{})
	if err == nil || !strings.Contains(err.Error(), "-to") {
		t.Fatalf("error = %v, want required -to", err)
	}
}

func TestParseOptionsUsesLoopbackDefaults(t *testing.T) {
	opts, err := parseOptions([]string{"-to", "private@inbound.test"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.smtpAddr != "127.0.0.1:2525" {
		t.Errorf("smtp address = %q", opts.smtpAddr)
	}
	if opts.from != "sample@sender.example" || opts.subject == "" {
		t.Errorf("defaults = %+v", opts)
	}
}

func TestParseOptionsRejectsHeaderInjection(t *testing.T) {
	_, err := parseOptions([]string{"-to", "private@inbound.test\r\nBcc: victim@example.test"})
	if err == nil {
		t.Fatal("expected invalid recipient error")
	}
}

func TestBuildSampleMessageIncludesRemoteAndEmbeddedImages(t *testing.T) {
	opts, err := parseOptions([]string{"-to", "private@inbound.test", "-subject", "A local issue"})
	if err != nil {
		t.Fatal(err)
	}
	message, err := buildSampleMessage(opts, time.Date(2026, 9, 20, 16, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	body := string(message)
	for _, want := range []string{
		"Subject: A local issue",
		"Content-Type: multipart/related",
		"src=\"cid:morgenblau-sample-image\"",
		"Content-Id: <morgenblau-sample-image>",
		"https://remote.example.invalid/newsletter.png",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("message missing %q:\n%s", want, body)
		}
	}
}

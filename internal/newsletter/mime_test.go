package newsletter

import (
	"strings"
	"testing"
)

func TestParseMIMEBlocksRemoteImagesAndKeepsInlineRasterAssets(t *testing.T) {
	raw := []byte("From: Example Weekly <hello@example.com>\r\n" +
		"Subject: A safe issue\r\n" +
		"Message-ID: <issue-1@example.com>\r\n" +
		"List-ID: Example Weekly <weekly.example.com>\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/related; boundary=rel\r\n\r\n" +
		"--rel\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
		`<p>Hello<script>alert(1)</script><img src="https://tracker.example/pixel" srcset="https://tracker.example/2x 2x"><img src="cid:logo"></p>` + "\r\n" +
		"--rel\r\nContent-Type: image/png\r\nContent-ID: <logo>\r\nContent-Transfer-Encoding: base64\r\n\r\niVBORw0KGgo=\r\n" +
		"--rel--\r\n")

	message := parseMIME(raw, "bounce@example.com")
	if message.SourceKey != "list-id:weekly.example.com" || message.SenderAddress != "hello@example.com" {
		t.Fatalf("identity = %q, %q", message.SourceKey, message.SenderAddress)
	}
	if strings.Contains(message.BodyHTMLBlocked, "tracker.example") || strings.Contains(message.BodyHTMLBlocked, "srcset") {
		t.Fatalf("blocked body can fetch remote image: %s", message.BodyHTMLBlocked)
	}
	if !strings.Contains(message.BodyHTMLRemote, "https://tracker.example/pixel") {
		t.Fatalf("remote body lost image: %s", message.BodyHTMLRemote)
	}
	if strings.Contains(message.BodyHTMLBlocked, "<script") || strings.Contains(message.BodyHTMLRemote, "<script") {
		t.Fatalf("script survived sanitization")
	}
	if len(message.Assets) != 1 || !strings.Contains(message.BodyHTMLBlocked, "/api/newsletter-assets/"+message.Assets[0].Token) {
		t.Fatalf("inline asset not rewritten: assets=%+v body=%s", message.Assets, message.BodyHTMLBlocked)
	}
	if !message.HasBlockedRemoteImages {
		t.Fatal("remote image was not marked blocked")
	}
}

func TestParseMIMEKeepsImageOnlyListMailAndInlineAsset(t *testing.T) {
	raw := []byte("From: Example Weekly <hello@example.com>\r\n" +
		"Subject: Illustrated issue\r\n" +
		"List-ID: Example Weekly <weekly.example.com>\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/related; boundary=rel\r\n\r\n" +
		"--rel\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
		`<img alt="Issue cover" src="cid:cover">` + "\r\n" +
		"--rel\r\nContent-Type: image/png\r\nContent-ID: <cover>\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\niVBORw0KGgo=\r\n" +
		"--rel--\r\n")

	message := parseMIME(raw, "bounce@example.com")
	if message.SourceKey != "list-id:weekly.example.com" {
		t.Fatalf("source key = %q", message.SourceKey)
	}
	if len(message.Assets) != 1 || !strings.Contains(message.BodyHTMLBlocked, "/api/newsletter-assets/"+message.Assets[0].Token) {
		t.Fatalf("inline image not preserved: assets=%+v body=%s", message.Assets, message.BodyHTMLBlocked)
	}
}

func TestParseMIMERemoteImageOnlyListMailKeepsConsentVariants(t *testing.T) {
	const remoteImage = "https://images.example.com/cover.png"
	raw := []byte("From: Example Weekly <hello@example.com>\r\n" +
		"Subject: Illustrated issue\r\n" +
		"List-ID: Example Weekly <weekly.example.com>\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/related; boundary=rel\r\n\r\n" +
		"--rel\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
		`<img alt="Issue cover" src="` + remoteImage + `">` + "\r\n" +
		"--rel--\r\n")

	message := parseMIME(raw, "bounce@example.com")
	if message.SourceKey != "list-id:weekly.example.com" {
		t.Fatalf("source key = %q", message.SourceKey)
	}
	if strings.Contains(message.BodyHTMLBlocked, remoteImage) || !strings.Contains(message.BodyHTMLRemote, remoteImage) {
		t.Fatalf("remote image variants = blocked %q, allowed %q", message.BodyHTMLBlocked, message.BodyHTMLRemote)
	}
	if !message.HasBlockedRemoteImages {
		t.Fatal("remote image was not marked blocked")
	}
}

func TestParseMIMEUsesStructuredForwardedMessageIdentity(t *testing.T) {
	raw := []byte("From: Alice <alice@example.net>\r\n" +
		"Subject: Fwd: Inner issue\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=mix\r\n\r\n" +
		"--mix\r\nContent-Type: text/plain\r\n\r\nSee attached.\r\n" +
		"--mix\r\nContent-Type: message/rfc822\r\nContent-Disposition: attachment\r\n\r\n" +
		"From: Inner Letter <letter@publisher.example>\r\nSubject: Inner issue\r\nMessage-ID: <inner-1@publisher.example>\r\nList-ID: Inner Letter <letter.publisher.example>\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nInner body.\r\n" +
		"--mix--\r\n")

	message := parseMIME(raw, "alice@example.net")
	if message.SourceKey != "list-id:letter.publisher.example" {
		t.Fatalf("source key = %q", message.SourceKey)
	}
	if message.SenderAddress != "letter@publisher.example" || message.Title != "Inner issue" || !strings.Contains(message.BodyText, "Inner body") {
		t.Fatalf("structured forward not selected: %+v", message)
	}
}

func TestParseMIMEFallsBackToReadableSafeContent(t *testing.T) {
	raw := []byte("From: broken@example.com\r\nSubject: Broken\r\nContent-Type: multipart/mixed; boundary=never\r\n\r\n" +
		"unclosed content <script>alert(1)</script> https://example.com")

	message := parseMIME(raw, "fallback@example.com")
	if message.BodyText == "" || message.BodyHTMLBlocked == "" {
		t.Fatalf("fallback was empty: %+v", message)
	}
	if strings.Contains(message.BodyHTMLBlocked, "<script") || strings.Contains(message.BodyHTMLBlocked, "<img") {
		t.Fatalf("unsafe fallback: %s", message.BodyHTMLBlocked)
	}
	if message.SenderAddress == "" || message.SourceKey == "" {
		t.Fatalf("fallback identity missing: %+v", message)
	}
}

func TestParseMIMEMultipleStructuredForwardsKeepOuterIdentity(t *testing.T) {
	raw := []byte("From: Alice <alice@example.net>\r\nSubject: Two forwards\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=mix\r\n\r\n" +
		"--mix\r\nContent-Type: text/plain\r\n\r\nMy notes.\r\n" +
		"--mix\r\nContent-Type: message/rfc822\r\n\r\nFrom: One <one@example.com>\r\nSubject: One\r\nContent-Type: text/plain\r\n\r\nFirst body.\r\n" +
		"--mix\r\nContent-Type: message/rfc822\r\n\r\nFrom: Two <two@example.com>\r\nSubject: Two\r\nContent-Type: text/plain\r\n\r\nSecond body.\r\n" +
		"--mix--\r\n")

	message := parseMIME(raw, "alice@example.net")
	if message.SourceKey != "from:alice@example.net" {
		t.Fatalf("source key = %q", message.SourceKey)
	}
	for _, content := range []string{"My notes", "First body", "Second body"} {
		if !strings.Contains(message.BodyText, content) {
			t.Fatalf("body missing %q: %s", content, message.BodyText)
		}
	}
}

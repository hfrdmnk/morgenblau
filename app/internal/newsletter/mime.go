package newsletter

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"html"
	"io"
	"mime"
	"net/mail"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-message"
	messageMail "github.com/emersion/go-message/mail"
	"github.com/microcosm-cc/bluemonday"
	"github.com/oklog/ulid/v2"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const (
	maxMIMEParts       = 200
	maxNestedMessages  = 4
	maxDecodedPartSize = 8 << 20
	maxInlineAssetSize = 2 << 20
)

type normalizedMail struct {
	SourceKey              string
	IdentityKind           string
	IdentityValue          string
	SourceTitle            string
	SenderName             *string
	SenderAddress          string
	MessageID              string
	Title                  string
	SentAt                 *time.Time
	BodyHTMLBlocked        string
	BodyHTMLRemote         string
	BodyText               string
	HasBlockedRemoteImages bool
	Assets                 []normalizedAsset
	AttachmentSummary      string
	DedupeContentHash      string
}

type normalizedAsset struct {
	Token       string
	ContentID   string
	MediaType   string
	Data        []byte
	ContentHash string
}

type attachmentMeta struct {
	Filename  string `json:"filename,omitempty"`
	MediaType string `json:"mediaType"`
}

func parseMIME(raw []byte, envelopeFrom string) normalizedMail {
	parsed, ok := parseMIMEAtDepth(raw, 0)
	if !ok {
		return fallbackMail(raw, envelopeFrom)
	}
	if parsed.SenderAddress == "" {
		parsed.SenderAddress = validAddress(envelopeFrom)
	}
	finalizeIdentity(&parsed, envelopeFrom)
	return parsed
}

func parseMIMEAtDepth(raw []byte, depth int) (normalizedMail, bool) {
	if depth > maxNestedMessages {
		return normalizedMail{}, false
	}
	reader, err := messageMail.CreateReader(bytes.NewReader(raw))
	if reader == nil || (err != nil && !message.IsUnknownCharset(err)) {
		return normalizedMail{}, false
	}
	defer reader.Close()

	result := normalizedMail{}
	result.Title, _ = reader.Header.Subject()
	result.MessageID, _ = reader.Header.MessageID()
	if sent, dateErr := reader.Header.Date(); dateErr == nil && !sent.IsZero() {
		sent = sent.UTC()
		result.SentAt = &sent
	}
	if from, fromErr := reader.Header.AddressList("From"); fromErr == nil && len(from) > 0 {
		result.SenderAddress = strings.ToLower(strings.TrimSpace(from[0].Address))
		if name := strings.TrimSpace(from[0].Name); name != "" {
			result.SenderName = &name
		}
	}
	listID := parseListID(reader.Header.Get("List-Id"))
	if listID != "" {
		result.IdentityKind = "list_id"
		result.IdentityValue = listID
	}

	var htmlBody string
	var plainBody string
	forwarded := make([]normalizedMail, 0)
	assetsByCID := make(map[string]normalizedAsset)
	attachments := make([]attachmentMeta, 0)
	for parts := 0; parts < maxMIMEParts; parts++ {
		part, partErr := reader.NextPart()
		if partErr == io.EOF {
			break
		}
		if part == nil || (partErr != nil && !message.IsUnknownCharset(partErr)) {
			continue
		}
		mediaType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		mediaType = strings.ToLower(mediaType)
		body, readErr := readBounded(part.Body, maxDecodedPartSize)
		if readErr != nil {
			continue
		}
		if mediaType == "message/rfc822" {
			if inner, innerOK := parseMIMEAtDepth(body, depth+1); innerOK {
				forwarded = append(forwarded, inner)
			}
			continue
		}
		switch header := part.Header.(type) {
		case *messageMail.InlineHeader:
			switch mediaType {
			case "text/html":
				if htmlBody == "" {
					htmlBody = string(body)
				}
			case "text/plain", "":
				if plainBody == "" {
					plainBody = string(body)
				}
			default:
				cid := normalizeContentID(header.Get("Content-Id"))
				if cid != "" && allowedInlineMediaType(mediaType) && len(body) <= maxInlineAssetSize {
					assetsByCID[cid] = makeAsset(cid, mediaType, body)
				}
			}
		case *messageMail.AttachmentHeader:
			cid := normalizeContentID(header.Get("Content-Id"))
			if cid != "" && allowedInlineMediaType(mediaType) && len(body) <= maxInlineAssetSize {
				assetsByCID[cid] = makeAsset(cid, mediaType, body)
				continue
			}
			filename, _ := header.Filename()
			attachments = append(attachments, attachmentMeta{Filename: strings.TrimSpace(filename), MediaType: mediaType})
		}
	}
	result.BodyText = strings.TrimSpace(plainBody)
	if htmlBody == "" {
		htmlBody = plainTextHTML(result.BodyText)
	}
	blocked, remote, hasRemote := sanitizeNewsletterHTML(htmlBody, assetsByCID)
	result.BodyHTMLBlocked = blocked
	result.BodyHTMLRemote = remote
	result.HasBlockedRemoteImages = hasRemote
	for _, asset := range assetsByCID {
		result.Assets = append(result.Assets, asset)
	}
	if len(attachments) > 0 {
		encoded, _ := json.Marshal(attachments)
		result.AttachmentSummary = string(encoded)
	}
	result.DedupeContentHash = contentDedupeHash(htmlBody, plainBody, assetsByCID, result.AttachmentSummary)
	if result.BodyText == "" {
		result.BodyText = strings.TrimSpace(bluemonday.StrictPolicy().Sanitize(htmlBody))
	}
	if len(forwarded) == 1 {
		inner := forwarded[0]
		inner.BodyText = joinReadable(result.BodyText, inner.BodyText)
		inner.BodyHTMLBlocked = joinReadableHTML(result.BodyHTMLBlocked, inner.BodyHTMLBlocked)
		inner.BodyHTMLRemote = joinReadableHTML(result.BodyHTMLRemote, inner.BodyHTMLRemote)
		inner.Assets = append(result.Assets, inner.Assets...)
		inner.HasBlockedRemoteImages = result.HasBlockedRemoteImages || inner.HasBlockedRemoteImages
		inner.DedupeContentHash = hashStrings(result.DedupeContentHash, inner.DedupeContentHash)
		return inner, true
	}
	if len(forwarded) > 1 {
		hashes := []string{result.DedupeContentHash}
		for _, inner := range forwarded {
			result.BodyText = joinReadable(result.BodyText, inner.BodyText)
			result.BodyHTMLBlocked = joinReadableHTML(result.BodyHTMLBlocked, inner.BodyHTMLBlocked)
			result.BodyHTMLRemote = joinReadableHTML(result.BodyHTMLRemote, inner.BodyHTMLRemote)
			result.Assets = append(result.Assets, inner.Assets...)
			result.HasBlockedRemoteImages = result.HasBlockedRemoteImages || inner.HasBlockedRemoteImages
			hashes = append(hashes, inner.DedupeContentHash)
		}
		result.DedupeContentHash = hashStrings(hashes...)
	}
	return result, result.BodyText != "" || strings.TrimSpace(bluemonday.StrictPolicy().Sanitize(result.BodyHTMLBlocked)) != "" ||
		result.HasBlockedRemoteImages || hasResolvedInlineAsset(result.BodyHTMLBlocked, result.Assets)
}

func hasResolvedInlineAsset(body string, assets []normalizedAsset) bool {
	for _, asset := range assets {
		if strings.Contains(body, "/api/newsletter-assets/"+asset.Token) {
			return true
		}
	}
	return false
}

func fallbackMail(raw []byte, envelopeFrom string) normalizedMail {
	sender := validAddress(envelopeFrom)
	title := "Received email"
	body := raw
	if parsed, err := mail.ReadMessage(bytes.NewReader(raw)); err == nil {
		if subject := strings.TrimSpace(parsed.Header.Get("Subject")); subject != "" {
			title = subject
		}
		if from, fromErr := mail.ParseAddress(parsed.Header.Get("From")); fromErr == nil {
			sender = strings.ToLower(from.Address)
		}
		if parsedBody, readErr := readBounded(parsed.Body, maxDecodedPartSize); readErr == nil {
			body = parsedBody
		}
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		text = "This email could not be decoded."
	}
	result := normalizedMail{Title: title, SenderAddress: sender, BodyText: text, BodyHTMLBlocked: plainTextHTML(text), BodyHTMLRemote: plainTextHTML(text), DedupeContentHash: hashStrings(text)}
	finalizeIdentity(&result, envelopeFrom)
	return result
}

func contentDedupeHash(htmlBody, plainBody string, assets map[string]normalizedAsset, attachments string) string {
	parts := []string{htmlBody, plainBody, attachments}
	contentIDs := make([]string, 0, len(assets))
	for contentID := range assets {
		contentIDs = append(contentIDs, contentID)
	}
	sort.Strings(contentIDs)
	for _, contentID := range contentIDs {
		asset := assets[contentID]
		parts = append(parts, contentID, asset.MediaType, asset.ContentHash)
	}
	return hashStrings(parts...)
}

func hashStrings(values ...string) string {
	digest := sha256.New()
	for _, value := range values {
		digest.Write([]byte(value))
		digest.Write([]byte{0})
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func finalizeIdentity(result *normalizedMail, envelopeFrom string) {
	if result.SenderAddress == "" {
		result.SenderAddress = validAddress(envelopeFrom)
	}
	if result.IdentityValue == "" {
		result.IdentityKind = "from"
		result.IdentityValue = result.SenderAddress
	}
	if result.IdentityValue == "" {
		result.IdentityValue = "unknown"
	}
	prefix := result.IdentityKind
	if prefix == "list_id" {
		prefix = "list-id"
	}
	result.SourceKey = prefix + ":" + strings.ToLower(result.IdentityValue)
	if result.SenderName != nil && strings.TrimSpace(*result.SenderName) != "" {
		result.SourceTitle = strings.TrimSpace(*result.SenderName)
	} else if result.SenderAddress != "" {
		result.SourceTitle = result.SenderAddress
	} else {
		result.SourceTitle = "Email"
	}
}

func sanitizeNewsletterHTML(raw string, assets map[string]normalizedAsset) (string, string, bool) {
	blocked, hasRemote := rewriteNewsletterHTML(raw, assets, false)
	remote, _ := rewriteNewsletterHTML(raw, assets, true)
	policy := bluemonday.UGCPolicy()
	return policy.Sanitize(blocked), policy.Sanitize(remote), hasRemote
}

func rewriteNewsletterHTML(raw string, assets map[string]normalizedAsset, allowRemote bool) (string, bool) {
	contextNode := &xhtml.Node{Type: xhtml.ElementNode, DataAtom: atom.Div, Data: "div"}
	nodes, err := xhtml.ParseFragment(strings.NewReader(raw), contextNode)
	if err != nil {
		return plainTextHTML(raw), false
	}
	hasRemote := false
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && node.Data == "img" {
			attrs := node.Attr[:0]
			for _, attr := range node.Attr {
				key := strings.ToLower(attr.Key)
				if key == "srcset" {
					continue
				}
				if key != "src" {
					attrs = append(attrs, attr)
					continue
				}
				src := strings.TrimSpace(attr.Val)
				if strings.HasPrefix(strings.ToLower(src), "cid:") {
					cid := normalizeContentID(src[4:])
					if asset, ok := assets[cid]; ok {
						attr.Val = "/api/newsletter-assets/" + asset.Token
						attrs = append(attrs, attr)
					}
					continue
				}
				parsed, parseErr := url.Parse(src)
				if parseErr == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
					hasRemote = true
					if allowRemote {
						attrs = append(attrs, attr)
					}
				}
			}
			node.Attr = attrs
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	var out strings.Builder
	for _, node := range nodes {
		walk(node)
		_ = xhtml.Render(&out, node)
	}
	return out.String(), hasRemote
}

func plainTextHTML(value string) string {
	return "<p>" + strings.ReplaceAll(html.EscapeString(value), "\n", "<br>") + "</p>"
}

func joinReadable(first, second string) string {
	first = strings.TrimSpace(first)
	second = strings.TrimSpace(second)
	if first == "" {
		return second
	}
	if second == "" {
		return first
	}
	return first + "\n\n" + second
}

func joinReadableHTML(first, second string) string {
	first = strings.TrimSpace(first)
	second = strings.TrimSpace(second)
	if first == "" || first == "<p></p>" {
		return second
	}
	if second == "" || second == "<p></p>" {
		return first
	}
	return first + "<hr>" + second
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, io.ErrUnexpectedEOF
	}
	return data, nil
}

func makeAsset(cid, mediaType string, data []byte) normalizedAsset {
	digest := sha256.Sum256(data)
	return normalizedAsset{Token: ulid.Make().String(), ContentID: cid, MediaType: mediaType, Data: data, ContentHash: hex.EncodeToString(digest[:])}
}

func allowedInlineMediaType(value string) bool {
	switch value {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func normalizeContentID(value string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(value), "<>"))
}

func parseListID(value string) string {
	value = strings.TrimSpace(value)
	if start := strings.LastIndex(value, "<"); start >= 0 {
		if end := strings.Index(value[start+1:], ">"); end >= 0 {
			value = value[start+1 : start+1+end]
		}
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.ContainsAny(value, " \t\r\n") {
		return ""
	}
	return value
}

func validAddress(value string) string {
	parsed, err := mail.ParseAddress(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Address)
}

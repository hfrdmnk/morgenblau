package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"os"
	"strings"
	"time"
)

type options struct {
	smtpAddr string
	to       string
	from     string
	subject  string
	emlPath  string
}

func parseOptions(args []string) (options, error) {
	fs := flag.NewFlagSet("send-newsletter", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	opts := options{}
	fs.StringVar(&opts.smtpAddr, "smtp", "127.0.0.1:2525", "SMTP host:port")
	fs.StringVar(&opts.to, "to", "", "private newsletter address")
	fs.StringVar(&opts.from, "from", "sample@sender.example", "SMTP envelope and From address")
	fs.StringVar(&opts.subject, "subject", "Morgenblau local newsletter sample", "sample message subject")
	fs.StringVar(&opts.emlPath, "eml", "", "send an existing .eml instead of the sample")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if strings.TrimSpace(opts.to) == "" {
		return options{}, errors.New("-to is required")
	}
	if strings.ContainsAny(opts.to+opts.from+opts.subject, "\r\n") {
		return options{}, errors.New("mail fields cannot contain newlines")
	}
	to, err := mail.ParseAddress(opts.to)
	if err != nil || to.Address != opts.to {
		return options{}, fmt.Errorf("invalid -to address")
	}
	from, err := mail.ParseAddress(opts.from)
	if err != nil || from.Address != opts.from {
		return options{}, fmt.Errorf("invalid -from address")
	}
	if !strings.Contains(opts.smtpAddr, ":") {
		return options{}, fmt.Errorf("invalid -smtp address")
	}
	return opts, nil
}

func buildSampleMessage(opts options, now time.Time) ([]byte, error) {
	var body bytes.Buffer
	related := multipart.NewWriter(&body)

	fmt.Fprintf(&body, "From: %s\r\n", opts.from)
	fmt.Fprintf(&body, "To: %s\r\n", opts.to)
	fmt.Fprintf(&body, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", opts.subject))
	fmt.Fprintf(&body, "Date: %s\r\n", now.Format(time.RFC1123Z))
	fmt.Fprintf(&body, "Message-ID: <%d.local-sample@morgenblau>\r\n", now.UnixNano())
	fmt.Fprint(&body, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&body, "Content-Type: multipart/related; boundary=%q\r\n\r\n", related.Boundary())

	htmlHeader := textproto.MIMEHeader{}
	htmlHeader.Set("Content-Type", "text/html; charset=utf-8")
	htmlHeader.Set("Content-Transfer-Encoding", "8bit")
	htmlPart, err := related.CreatePart(htmlHeader)
	if err != nil {
		return nil, err
	}
	_, err = io.WriteString(htmlPart, `<!doctype html>
<html><body>
<h1>A local Morgenblau issue</h1>
<p>This fixture contains one embedded image and one blocked remote image.</p>
<img src="cid:morgenblau-sample-image" alt="Embedded green pixel" width="24" height="24">
<img src="https://remote.example.invalid/newsletter.png" alt="Remote image placeholder" width="240" height="120">
</body></html>`)
	if err != nil {
		return nil, err
	}

	imageHeader := textproto.MIMEHeader{}
	imageHeader.Set("Content-Type", "image/png")
	imageHeader.Set("Content-Transfer-Encoding", "base64")
	imageHeader.Set("Content-ID", "<morgenblau-sample-image>")
	imageHeader.Set("Content-Disposition", `inline; filename="sample.png"`)
	imagePart, err := related.CreatePart(imageHeader)
	if err != nil {
		return nil, err
	}
	pixel, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	if err != nil {
		return nil, err
	}
	encoded := base64.NewEncoder(base64.StdEncoding, imagePart)
	if _, err := encoded.Write(pixel); err != nil {
		return nil, err
	}
	if err := encoded.Close(); err != nil {
		return nil, err
	}
	if err := related.Close(); err != nil {
		return nil, err
	}
	return body.Bytes(), nil
}

func messageForOptions(opts options, now time.Time) ([]byte, error) {
	if opts.emlPath != "" {
		message, err := os.ReadFile(opts.emlPath)
		if err != nil {
			return nil, fmt.Errorf("read eml: %w", err)
		}
		return message, nil
	}
	return buildSampleMessage(opts, now)
}

func run(args []string) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	message, err := messageForOptions(opts, time.Now())
	if err != nil {
		return err
	}
	if err := smtp.SendMail(opts.smtpAddr, nil, opts.from, []string{opts.to}, message); err != nil {
		return fmt.Errorf("send newsletter: %w", err)
	}
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

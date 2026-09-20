package newsletter

import (
	"context"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSMTPAcceptsMultipleOwnerRecipientsOnlyAfterAtomicReceiptCommit(t *testing.T) {
	service, reader, _ := newTestService(t)
	ctx := context.Background()
	alice, _ := service.Address(ctx, "did:plc:alice")
	bob, _ := service.Address(ctx, "did:plc:bob")
	server, client := serveSMTP(t, service)
	if err := client.Hello("sender.example"); err != nil {
		t.Fatal(err)
	}
	if err := client.Mail("bounce@sender.example"); err != nil {
		t.Fatal(err)
	}
	if err := client.Rcpt(alice); err != nil {
		t.Fatal(err)
	}
	if err := client.Rcpt(bob); err != nil {
		t.Fatal(err)
	}
	data, err := client.Data()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.Write([]byte("From: Sender <sender@example.com>\r\nSubject: One\r\n\r\nBody\r\n")); err != nil {
		t.Fatal(err)
	}
	if err := data.Close(); err != nil {
		t.Fatal(err)
	}
	client.Quit()
	server.Close()

	if got := countRows(t, reader, "newsletter_receipts"); got != 2 {
		t.Fatalf("receipt count after SMTP 250 = %d, want 2", got)
	}
}

func TestSMTPRejectsUnknownRecipient(t *testing.T) {
	service, _, _ := newTestService(t)
	_, client := serveSMTP(t, service)
	defer client.Quit()
	client.Hello("sender.example")
	client.Mail("bounce@sender.example")
	err := client.Rcpt("unknown@news.example")
	if err == nil || !strings.HasPrefix(err.Error(), "550") {
		t.Fatalf("RCPT error = %v, want 550", err)
	}
}

func TestSMTPReturnsTemporaryFailureWhenReceiptCannotCommit(t *testing.T) {
	service, _, writer := newTestService(t)
	ctx := context.Background()
	address, _ := service.Address(ctx, "did:plc:alice")
	_, client := serveSMTP(t, service)
	defer client.Quit()
	client.Hello("sender.example")
	client.Mail("bounce@sender.example")
	if err := client.Rcpt(address); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := client.Data()
	if err != nil {
		t.Fatal(err)
	}
	data.Write([]byte("From: sender@example.com\r\nSubject: Retry\r\n\r\nBody\r\n"))
	err = data.Close()
	if err == nil || !strings.HasPrefix(err.Error(), "451") {
		t.Fatalf("DATA error = %v, want 451", err)
	}
}

func TestSMTPReturnsTemporaryFailureBeforeAcceptingOverQuota(t *testing.T) {
	service, reader, _ := newTestServiceWithConfig(t, Config{
		Domain: "news.example", OwnerStorageBytes: defaultReceiptReservationBytes - 1,
		GlobalStorageBytes: defaultGlobalStorageBytes,
	})
	ctx := context.Background()
	address, _ := service.Address(ctx, "did:plc:alice")
	_, client := serveSMTP(t, service)
	defer client.Quit()
	client.Hello("sender.example")
	client.Mail("bounce@sender.example")
	if err := client.Rcpt(address); err != nil {
		t.Fatal(err)
	}
	data, err := client.Data()
	if err != nil {
		t.Fatal(err)
	}
	data.Write([]byte("From: sender@example.com\r\nSubject: quota\r\n\r\nBody\r\n"))
	err = data.Close()
	if err == nil || !strings.HasPrefix(err.Error(), "451") {
		t.Fatalf("DATA error = %v, want 451", err)
	}
	if got := countRows(t, reader, "newsletter_receipts"); got != 0 {
		t.Fatalf("receipt count = %d, want 0", got)
	}
}

func TestLimitedSMTPListenerCapsConnectionsBeforeGreeting(t *testing.T) {
	service, _, _ := newTestService(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	limited := LimitSMTPListener(listener, 1)
	server := NewSMTPServer(service, SMTPConfig{Domain: "news.example", MaxConnections: 1})
	done := make(chan error, 1)
	go func() { done <- server.Serve(limited) }()
	t.Cleanup(func() {
		server.Close()
		<-done
	})

	first, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := second.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 64)
	if _, err := second.Read(buffer); err == nil {
		t.Fatal("second connection received greeting before the first connection closed")
	} else if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
		t.Fatalf("second read error = %v, want timeout", err)
	}
	first.Close()
	if err := second.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if n, err := second.Read(buffer); err != nil || !strings.HasPrefix(string(buffer[:n]), "220 ") {
		t.Fatalf("second greeting = %q, %v", string(buffer[:n]), err)
	}
}

func serveSMTP(t *testing.T, service *Service) (*smtpServerHandle, *smtp.Client) {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	listener := newPipeListener(serverConn)
	server := NewSMTPServer(service, SMTPConfig{Domain: "news.example", MaxMessageBytes: 1 << 20, MaxConnections: 8})
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	client, err := smtp.NewClient(clientConn, "news.example")
	if err != nil {
		t.Fatal(err)
	}
	handle := &smtpServerHandle{close: func() {
		server.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("SMTP server did not stop")
		}
	}}
	t.Cleanup(handle.Close)
	return handle, client
}

type smtpServerHandle struct {
	close func()
}

func (h *smtpServerHandle) Close() {
	if h.close != nil {
		h.close()
		h.close = nil
	}
}

type pipeListener struct {
	connections chan net.Conn
	closed      chan struct{}
	closeOnce   sync.Once
}

func newPipeListener(connection net.Conn) *pipeListener {
	connections := make(chan net.Conn, 1)
	connections <- connection
	return &pipeListener{connections: connections, closed: make(chan struct{})}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case connection := <-l.connections:
		return connection, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddress("memory") }

type pipeAddress string

func (a pipeAddress) Network() string { return "pipe" }
func (a pipeAddress) String() string  { return string(a) }

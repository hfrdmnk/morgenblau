package scripts

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeShell(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nset -eu\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestEntrypointRestorePublication(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mode      string
		bootstrap bool
		existing  bool
		abandoned bool
		wantError bool
	}{
		{name: "validated restore", mode: "valid"},
		{name: "abandoned staging ignored", mode: "valid", abandoned: true},
		{name: "failed restore retried", mode: "fail", wantError: true},
		{name: "empty replica denied", mode: "empty", wantError: true},
		{name: "empty replica bootstrapped", mode: "empty", bootstrap: true},
		{name: "failed restore denied during bootstrap", mode: "fail", bootstrap: true, wantError: true},
		{name: "existing database preserved", mode: "fail", existing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "bin")
			data := filepath.Join(dir, "data with spaces")
			for _, path := range []string{bin, data} {
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			db := filepath.Join(data, "app.db")
			log := filepath.Join(dir, "calls")
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("TEST_BIN", bin)
			t.Setenv("TEST_LOG", log)
			t.Setenv("RESTORE_MODE", tc.mode)
			t.Setenv("APP_ENV", "production")
			t.Setenv("DB_PATH", db)
			certDir := filepath.Join(data, "certmagic")
			t.Setenv("SMTP_ACME_ENABLED", "true")
			t.Setenv("SMTP_ACME_STORAGE", certDir)
			t.Setenv("LITESTREAM_REPLICA_URL", "file:///unused-fixture")
			t.Setenv("LITESTREAM_ALLOW_EMPTY_REPLICA", "false")
			if tc.bootstrap {
				t.Setenv("LITESTREAM_ALLOW_EMPTY_REPLICA", "true")
			}
			if tc.existing {
				if err := os.WriteFile(db, []byte("existing"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.abandoned {
				staging := filepath.Join(data, ".restore.abandoned")
				if err := os.Mkdir(staging, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(staging, "database.db"), []byte("unvalidated"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			writeShell(t, bin, "chown", "exit 0\n")
			writeShell(t, bin, "gosu", `
[ "$1" = nobody ]
shift
command="$1"
shift
case "$command" in
  /app/*) exec "$TEST_BIN/${command##*/}" "$@" ;;
  *) exec "$command" "$@" ;;
esac
`)
			writeShell(t, bin, "litestream", `
if [ "$1" = replicate ]; then
  printf 'replicate\n' >> "$TEST_LOG"
  exit 0
fi
[ "$1" = restore ]
shift
output="$DB_PATH"
skip=false
bootstrap=false
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) output="$2"; shift ;;
    -config|-integrity-check) shift ;;
    -if-db-not-exists) skip=true ;;
    -if-replica-exists) bootstrap=true ;;
  esac
  shift
done
if [ "$skip" = true ] && [ -e "$output" ]; then exit 0; fi
printf 'restore\n' >> "$TEST_LOG"
case "$RESTORE_MODE" in
  valid) printf 'validated' > "$output" ;;
  fail) printf 'unvalidated' > "$output"; exit 42 ;;
  empty) [ "$bootstrap" = true ] || exit 43 ;;
esac
`)
			writeShell(t, bin, "goose", `
printf 'migrate\n' >> "$TEST_LOG"
if [ ! -e "$DB_PATH" ]; then printf 'bootstrap' > "$DB_PATH"; fi
`)
			attempts := 1
			if tc.wantError {
				attempts = 2
			}
			for attempt := range attempts {
				out, err := exec.Command("sh", "container-entrypoint.sh").CombinedOutput()
				if (err != nil) != tc.wantError {
					t.Fatalf("attempt %d: error = %v, wantError = %v; output: %s", attempt+1, err, tc.wantError, out)
				}
			}
			calls, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantError {
				if string(calls) != "restore\nrestore\n" {
					t.Fatalf("failed restore reached migrations or was not retried: %s", calls)
				}
				if _, err := os.Stat(db); !os.IsNotExist(err) {
					t.Fatalf("failed restore published live database: %v", err)
				}
			} else {
				wantCalls, wantDB := "restore\nmigrate\nreplicate\n", "validated"
				if tc.existing {
					wantCalls, wantDB = "migrate\nreplicate\n", "existing"
				} else if tc.mode == "empty" {
					wantDB = "bootstrap"
				}
				contents, err := os.ReadFile(db)
				if err != nil || string(contents) != wantDB || string(calls) != wantCalls {
					t.Fatalf("database = %q (%v), calls = %q; want %q, %q", contents, err, calls, wantDB, wantCalls)
				}
				info, err := os.Stat(certDir)
				if err != nil || info.Mode().Perm() != 0o700 {
					t.Fatalf("certificate storage: %v %v", info, err)
				}
			}
			staging, err := filepath.Glob(filepath.Join(data, ".restore.*"))
			wantStaging := 0
			if tc.abandoned {
				wantStaging = 1
			}
			if err != nil || len(staging) != wantStaging {
				t.Fatalf("staging directories not cleaned: %v (%v)", staging, err)
			}
		})
	}
}

func TestSMTPSecretsUpload(t *testing.T) {
	cert, key := strings.Repeat("certificate fixture\n", 10), "different private key fixture\n"
	for _, tc := range []struct {
		name       string
		missing    string
		empty      string
		encoder    string
		uploadFail bool
	}{
		{name: "fullchain and key uploaded"},
		{name: "missing certificate", missing: "fullchain.pem"},
		{name: "missing key", missing: "privkey.pem"},
		{name: "empty certificate", empty: "fullchain.pem"},
		{name: "empty key", empty: "privkey.pem"},
		{name: "encoder failure", encoder: "printf partial; exit 23\n"},
		{name: "key encoder failure", encoder: `
if [ -e "$RENEWED_LINEAGE/encoded-once" ]; then printf partial; exit 23; fi
touch "$RENEWED_LINEAGE/encoded-once"
printf encoded-certificate
`},
		{name: "empty encoder output", encoder: "exit 0\n"},
		{name: "upload failure", uploadFail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "bin")
			if err := os.Mkdir(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			for name, contents := range map[string]string{"fullchain.pem": cert, "privkey.pem": key} {
				if name == tc.missing {
					continue
				}
				if name == tc.empty {
					contents = ""
				}
				if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			capture := filepath.Join(dir, "upload")
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("TMPDIR", dir)
			t.Setenv("FLY_APP", "fixture-app")
			t.Setenv("RENEWED_LINEAGE", dir)
			t.Setenv("TEST_CAPTURE", capture)
			t.Setenv("UPLOAD_EXIT", "0")
			if tc.uploadFail {
				t.Setenv("UPLOAD_EXIT", "24")
			}
			writeShell(t, bin, "fly", `
[ "$*" = 'secrets import --app fixture-app' ]
cat > "$TEST_CAPTURE"
exit "$UPLOAD_EXIT"
`)
			if tc.encoder != "" {
				writeShell(t, bin, "base64", tc.encoder)
			}
			out, err := exec.Command("sh", "update-fly-smtp-cert.sh").CombinedOutput()
			invalid := tc.missing != "" || tc.empty != "" || tc.encoder != ""
			if (err != nil) != (invalid || tc.uploadFail) {
				t.Errorf("error = %v; invalid = %v, uploadFail = %v; output: %s", err, invalid, tc.uploadFail, out)
			}
			payload, readErr := os.ReadFile(capture)
			if invalid {
				if !os.IsNotExist(readErr) {
					t.Errorf("invalid certificate invoked Fly: %q (%v)", payload, readErr)
				}
			} else {
				want := "SMTP_TLS_CERT_B64=" + base64.StdEncoding.EncodeToString([]byte(cert)) + "\nSMTP_TLS_KEY_B64=" + base64.StdEncoding.EncodeToString([]byte(key)) + "\n"
				if readErr != nil || string(payload) != want {
					t.Errorf("payload = %q (%v), want %q", payload, readErr, want)
				}
			}
			files, err := filepath.Glob(filepath.Join(dir, "tmp.*"))
			if err != nil || len(files) != 0 {
				t.Fatalf("secret tempfile not cleaned: %v (%v)", files, err)
			}
		})
	}
}

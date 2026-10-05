# Deploy the private alpha on Fly.io

Use one always-on Fly Machine, a local SQLite volume, and Litestream backups in a **private** Tigris bucket. Fly suits this Go binary and raw inbound SMTP; Laravel Cloud is not the natural fit for this stack. A single Machine means brief downtime during deploys, not high availability.

`fly.toml` owns sizing, region, ports, health checks, and public configuration. `scripts/container-entrypoint.sh` owns restore → migrations → supervised app startup. `litestream.yml` owns backup timing and retention. Do not add a Fly `release_command`: release Machines do not mount the application's volume.

## Domain plan

| Name | Purpose |
| --- | --- |
| `morgen.blue` | Landing page, hosted separately; existing human email stays untouched |
| `app.morgen.blue` | App, OAuth metadata, and callback |
| `newsletters.morgen.blue` | Private receiving addresses, e.g. `<random>@newsletters.morgen.blue` |
| `mx.app.morgen.blue` | Product SMTP banner, STARTTLS certificate, and MX target |

The app subdomain keeps landing-page hosting independent and avoids routing OAuth and the PWA through a subpath. The SMTP hostname is infrastructure, not an address suffix; other receiving domains can point their MX records at it later, but the app must also be taught to accept them. Adding DNS alone does not enable another receiving domain.

Future outgoing product mail can use `notify.morgen.blue` for transactional messages and `updates.morgen.blue` for marketing, through an email provider with its own SPF/DKIM/DMARC configuration. Neither needs to use the receiving server. Keep human email on `@morgen.blue`; do not replace the root's MX records.

## 1. Create the app and persistent storage

Install [flyctl](https://fly.io/docs/flyctl/install/), sign in, and run these commands from this repository. These commands create billable resources; choose your Fly organization when prompted.

```sh
fly auth login
export FLY_APP=your-globally-unique-app-name
fly apps create "$FLY_APP"
fly volumes create morgenblau_data --region fra --size 3 --app "$FLY_APP"
fly ips allocate-v4 --app "$FLY_APP"
fly ips allocate-v6 --app "$FLY_APP"
fly storage create --app "$FLY_APP"
```

Set `app` in `fly.toml` to that name. Leave the bucket **private**; it holds newsletter content and encrypted OAuth sessions. Save the Tigris credentials in your password manager when they are printed: Fly cannot show their original values later. `fly storage create` attaches bucket credentials to the app automatically.

The dedicated IPv4 is needed for raw TCP port 25, not just HTTP. Start with the committed 512 MB size; watch memory usage and increase it to 1024 MB if necessary. Check [Fly pricing](https://fly.io/docs/about/pricing/) and [Tigris pricing](https://www.tigrisdata.com/docs/pricing/) for the selected organization/region. Budget for the always-on VM, volume, dedicated IPv4, snapshots, and object storage; this is not a free-tier deployment.

Keep **exactly one Machine**. Volumes are local, not shared or automatically synchronized. Do not enable autoscaling, blue-green deploys, or a second independent writer using the same backup prefix.

## 2. Prepare the secrets file

Fly injects environment variables from secrets; you do not need a `.env` file inside the server. Keep a private `.env.production` locally for initial import. It is gitignored and excluded from Docker build context. **Do not copy your development `.env` wholesale**: that could disable alpha mode or override production OAuth URLs.

Generate fresh production keys directly into the file without putting their values in shell history:

```sh
umask 077
test ! -e .env.production || { echo '.env.production already exists; preserve its keys'; exit 1; }
{
  printf 'SESSION_COOKIE_KEY='; openssl rand -base64 32
  printf 'SESSION_STORE_KEYS='; openssl rand -base64 32
  printf 'BLUESKY_OAUTH_PRIVATE_KEY='
  openssl ecparam -name prime256v1 -genkey -noout \
    | openssl pkcs8 -topk8 -nocrypt | openssl base64 -A
  printf '\n'
} > .env.production
chmod 600 .env.production
```

Append these lines in your editor, replacing the DID and bucket placeholders. Values in a Fly secrets import file are **unquoted**; the comma-separated allowlist may have spaces between entries, but not a trailing comma.

```dotenv
ALPHA_ALLOWED_DIDS=did:plc:your-account-did,did:plc:another-invited-account
LITESTREAM_REPLICA_URL=s3://your-private-bucket/morgenblau?endpoint=t3.storage.dev&region=auto
LITESTREAM_ALLOW_EMPTY_REPLICA=true
SMTP_CERT_CONTACT_EMAIL=you@example.com
```

Use your account's stable DID, not its handle. You still sign in with your handle normally. `ALPHA_ENABLED=true` is already in `fly.toml`; an empty or malformed list then refuses startup. The callback checks the verified DID before setting a cookie or dispatching sync. Existing sessions are checked before resuming, so removing a DID takes effect after the Machine restarts.

The `endpoint=` URL parameter is intentional: setting only `AWS_ENDPOINT_URL_S3` bypasses Litestream's [Tigris-specific detection](https://litestream.io/guides/tigris/). `fly storage create` may set that extra variable, but our explicit URL supplies the correct endpoint.

Back up the production encryption/signing keys separately from SQLite. Restoring the database without `SESSION_STORE_KEYS` cannot recover its encrypted sessions. Do not regenerate keys on each deploy.

```sh
fly secrets import --stage --app "$FLY_APP" < .env.production
```

**Bootstrap is one-time only.** `LITESTREAM_ALLOW_EMPTY_REPLICA=true` permits a first database when the bucket is empty. Remove it after the backup check in step 5. Normally an absent local database must restore successfully before migrations run; inaccessible, empty, or broken backups must not silently become a new empty app.

For an RSS-only deployment, set `NEWSLETTER_DOMAIN=""`, `SMTP_LISTEN_ADDR=""`, and `SMTP_ACME_ENABLED="false"` in `fly.toml`. Complete step 3 and restore those settings before expecting inbound newsletters to work. You can defer the dedicated IPv4 until then.

## 3. Configure DNS and the SMTP certificate

Run `fly ips list --app "$FLY_APP"`. In Cloudflare's **DNS → Records** for `morgen.blue`, configure these records using the actual allocated addresses:

| Type | Name | Value | Proxy status |
| --- | --- | --- | --- |
| A | `app.morgen.blue` | Dedicated Fly IPv4 | DNS only (grey cloud) |
| AAAA | `app.morgen.blue` | Fly IPv6 | DNS only (grey cloud) |
| A | `mx.app.morgen.blue` | Dedicated Fly IPv4 | DNS only (grey cloud) |
| MX | `newsletters.morgen.blue` | Priority 10, `mx.app.morgen.blue` | Not applicable |

Start with the app records DNS-only too, so Fly handles HTTPS directly without Cloudflare challenges interfering with OAuth. Cloudflare's normal proxy **does not proxy SMTP**. An MX target must resolve directly to address records, not a CNAME. Publish no SMTP AAAA record until you have tested that path. Leave the landing page's proxy setting and root email records alone.

Provision the HTTPS certificate for the app:

```sh
fly certs add app.morgen.blue --app "$FLY_APP"
fly certs check app.morgen.blue --app "$FLY_APP"
```

Follow any verification records Fly requests. Fly terminates HTTPS at its proxy, but **cannot substitute that certificate for the application's SMTP STARTTLS certificate**. The port 25 service intentionally has no TLS handler: SMTP starts in plaintext and upgrades via STARTTLS.

Morgenblau embeds [CertMagic](https://github.com/caddyserver/certmagic) to obtain and renew the publicly trusted Let's Encrypt certificate for **`mx.app.morgen.blue`**. `internal/server/smtp_certificates.go` owns issuance, renewal, readiness and shutdown. No Certbot host, Cloudflare API token, certificate-upload deploy token, or renewal timer is needed.

`SMTP_ACME_ENABLED="true"` in `fly.toml` enables this path and signifies acceptance of [Let's Encrypt's subscriber agreement](https://letsencrypt.org/repository/). Set `SMTP_CERT_CONTACT_EMAIL` to an address you monitor. Leave all `SMTP_TLS_*` keypair settings unset; automatic and static TLS configurations conflict and refuse startup.

Validation is **HTTP-01 only**. Public port 80 for `mx.app.morgen.blue` must reach the Go server on port 8000, preserving the host and challenge path. The dedicated IPv4 also routes HTTP without a Fly certificate for that hostname. Keep Fly managing **only `app.morgen.blue`**; do not add a Fly HTTPS certificate for the SMTP hostname. Cloudflare must remain DNS-only, and any CAA policy must permit Let's Encrypt. TLS-ALPN-01 and DNS-01 are not used.

Do not enable Fly's `force_https` on port 80: the app serves challenges before authentication and redirects. Ordinary HTTP requests redirect to the configured app HTTPS host; forwarded HTTPS is trusted only when `FLY_APP_NAME` indicates the established Fly proxy boundary. Do not expose backend port 8000 directly to untrusted clients. The anonymous database health route remains available on HTTP so Fly can route challenges **before** issuance finishes.

After HTTP starts listening, the app proactively provisions the single configured `SMTP_HOSTNAME`; request Host/SNI never triggers issuance for other names. SMTP sends no greeting until a valid certificate is available. Renewed certificates replace the active STARTTLS certificate without a deploy, including for clients without SNI. Unexpected SNI and expired certificates fail TLS; new SMTP connections wait while no valid certificate exists. A usable cached certificate stays available during renewal failures. Already established SMTP sessions are not forcibly terminated at expiry; SMTP remains an opportunistic STARTTLS receiver, not a TLS-required submission service.

Certificate/account private keys live in `SMTP_ACME_STORAGE` (default `/data/certmagic`), a private persistent directory owned by the unprivileged app. The worker checks renewal each minute; failed attempts are time-bounded and retried with exponential backoff from one minute to at most six hours while HTTP stays up. Watch `SMTP certificate ready` and `SMTP certificate maintenance failed; retrying` logs and monitor the served expiry independently: database health does not prove SMTP readiness.

SMTP TLS session resumption is disabled so every STARTTLS handshake checks the current identity and validity. An interrupted renewal's unusable keypair triggers a forced renewal without deleting the ACME account; storage-access failures remain failures, not a reason to discard keys. CertMagic's initially stored ACME Renewal Information (ARI) participates in scheduling, but this worker does not periodically refresh CA advice or detect revocation: later emergency/accelerated replacement instructions are not automatically picked up. Shutdown joins app-owned certificate work; CertMagic's process-global rate limiter and transient file-lock refreshers are not a fully stoppable embedded runtime and end with the process.

**Litestream backs up SQLite, not certificate storage.** The volume survives deploys, and volume snapshots may recover certificate assets. If the volume is lost and only SQLite is restored, the app creates a new ACME account/certificate after DNS/port 80 work again. Mail is unavailable until issuance succeeds; CA rate limits or outages can prolong this. Keep an encrypted backup of `/data/certmagic` if you need to recover the existing keys/account; preserve owner and private permissions when restoring it. Never delete the cache as a routine renewal procedure.

Explicit static PEM files or base64 PEM secrets remain supported with `SMTP_ACME_ENABLED="false"`. These require your own renewal and an app restart to load replacements; production startup checks identity and validity. `scripts/update-fly-smtp-cert.sh` is an optional legacy static upload helper, not part of normal deployment. When migrating an existing deployment to automatic mode, remove its static certificate secrets in the same planned deployment window and retire the old external renewal hook/tokens so they cannot reintroduce conflicting settings.

## 4. Deploy one Machine

```sh
fly config validate
fly deploy --app "$FLY_APP" --ha=false
fly scale count 1 --app "$FLY_APP"
fly status --app "$FLY_APP"
fly checks list --app "$FLY_APP"
fly logs --app "$FLY_APP"
```

`--ha=false` prevents Fly's default redundant Machine creation on the initial deploy. Subsequent deploys should still leave exactly one Machine. Its volume persists; pending goose migrations run on that volume before HTTP/SMTP starts. There are no automatic schema downs on rollback.

The app and Litestream run unprivileged. SIGTERM stops the app first and then attempts a final backup sync; `fly.toml` allows time for both. Hard kills, storage outages, and host failures can still lose changes not yet replicated. Litestream is asynchronous backup, not synchronous replication or automatic failover.

## 5. Verify access, mail, and an actual restore

```sh
curl -fsS https://app.morgen.blue/api/health
curl -fsS https://app.morgen.blue/oauth-client-metadata.json
curl -fsS https://app.morgen.blue/oauth-jwks.json
curl -sS -o /dev/null -w '%{http_code}\n' https://app.morgen.blue/api/digest
```

Expect `{"status":"up"}`, public metadata/JWKS, and `401` for anonymous digest access. Open the app in your browser and sign in with an allowed account. An uninvited account must receive `403` after OAuth, with no admitted session. Do not put a blanket password/challenge in front of OAuth metadata or callbacks: the authorization server must be able to reach them. The DID gate protects app data; the sign-in page, assets, and public about page remain public.

For newsletters, create your private receiving address in the app, subscribe a test newsletter, and confirm its message appears. Check STARTTLS from your computer:

```sh
openssl s_client -starttls smtp -connect mx.app.morgen.blue:25 \
  -servername mx.app.morgen.blue -verify_hostname mx.app.morgen.blue \
  -verify_return_error </dev/null
openssl s_client -starttls smtp -connect mx.app.morgen.blue:25 \
  -noservername -verify_hostname mx.app.morgen.blue -verify_return_error </dev/null
```

Expect successful certificate verification in both cases. Also check that `http://app.morgen.blue/` redirects to HTTPS while `/api/health` remains healthy during provisioning. An unrecognized challenge token is not an issuance test; actual HTTP-01 validation is performed by the CA. If your local ISP blocks outbound port 25, test from another network. The product is a receiver, not an outgoing mail relay.

Force a remote sync and restore to a **different path** on the Machine:

```sh
fly ssh console --app "$FLY_APP" -C \
  '/app/litestream sync -socket /data/litestream.sock -wait /data/morgenblau.db'
fly ssh console --app "$FLY_APP" -C \
  '/app/litestream restore -integrity-check full -o /tmp/backup-check.db /data/morgenblau.db'
```

Both must succeed; a process running or a bucket existing does not prove a backup is usable. The restored file contains private data. Delete it after the check, then disable empty bootstrap:

```sh
fly ssh console --app "$FLY_APP" -C 'rm /tmp/backup-check.db'
fly secrets unset LITESTREAM_ALLOW_EMPTY_REPLICA --app "$FLY_APP"
```

Remove that line from your local `.env.production` too, so a future import does not re-enable it. For subsequent restores, use another unique output filename or remove the previous *test* file first; never use `-force` against the live DB.

For backup-process failure alerts, create a dead-man's-switch check (e.g. Healthchecks.io), set its interval to 5 minutes with suitable grace, and import its URL as `LITESTREAM_HEARTBEAT_URL`. An idle sync can use cached replica state without contacting storage, so a heartbeat does not prove the bucket is reachable or a backup recoverable. With or without heartbeat alerts, inspect replication errors and repeat the restore drill regularly; HTTP health checks cover the database, not remote backup health.

## Operating the alpha

- **Invite/revoke:** edit only the `ALPHA_ALLOWED_DIDS` line in a private import file, then `fly secrets import --app "$FLY_APP" < <that-file>`. This restarts the Machine. Revoked accounts lose app access, but their stored data is not deleted.
- **Public launch:** deliberately set `ALPHA_ENABLED="false"` in `fly.toml` and deploy. Do not clear the list while alpha mode is on.
- **Updates:** first verify a remote sync/restore, then deploy. Keep an immutable reference to the previous image (`fly releases --image --app "$FLY_APP"`). Test migrations against a restored disposable DB before applying future schema changes.
- **Image rollback:** `fly deploy --app "$FLY_APP" --ha=false --image <previous-image>` changes code, not SQLite schema or data. Only use it when that image supports the current schema. Never run `goose down` automatically in production.
- **Disk:** inspect `df -h /data` over `fly ssh console`. Grow the volume with Fly's volume extension command before it fills; database, WAL, and Litestream metadata all need headroom. Newsletter quotas live in `internal/newsletter/ingest.go`; feed storage also grows.
- **Secrets:** rotating `SESSION_COOKIE_KEY` signs everyone out. For `SESSION_STORE_KEYS`, prepend a new key and retain old decrypting keys until sessions have been re-saved. Keep recoverable copies of all keys still needed by retained backups.

## Recovery and point-in-time rollback

If a Machine dies but its volume survives, restart/redeploy using that volume. If the volume is lost, follow Fly's [volume recovery guidance](https://fly.io/docs/volumes/overview/) to attach a new empty volume to the **single** replacement Machine. Keep the original stopped. With the same keys and replica URL, our entrypoint restores the latest backup before migrations. Do **not** re-enable empty bootstrap during recovery. Fly volume snapshots are a second recovery source, not a substitute for tested Litestream backups.

For a point-in-time restore, install the same Litestream version pinned in `Dockerfile` on your trusted recovery computer and supply the Tigris credentials privately via environment variables. Restore to a new file, not over the live DB:

```sh
litestream restore -dry-run -timestamp <UTC-RFC3339-time> \
  -o recovery.db 's3://your-private-bucket/morgenblau?endpoint=t3.storage.dev&region=auto'
litestream restore -integrity-check full -timestamp <UTC-RFC3339-time> \
  -o recovery.db 's3://your-private-bucket/morgenblau?endpoint=t3.storage.dev&region=auto'
sqlite3 recovery.db 'PRAGMA foreign_key_check;'
```

Foreign-key checking must return no rows. Available restore times are limited by the retained Litestream files; inspect the dry-run plan rather than assuming arbitrary per-transaction rollback. Retention and restore granularity are configured in `litestream.yml`.

Treat replacement of a populated volume as maintenance: stop the app and replication, preserve the original DB **and WAL/SHM/metadata**, install the verified restored file using a maintenance Machine with no app writer, and use a **new backup prefix** before starting it. Restored files must be owned by `nobody:nogroup`, matching the unprivileged app and backup process. Never run two writers or upload a restored older DB into the active replica history. Deploy code compatible with the restored schema. This operation can discard newer private newsletters/saves and needs an explicit decision; it does not roll back PDS records, which will reconcile on sign-in.

## Local certificate verification

Never request real Let's Encrypt certificates to test the app. The opt-in `TestSMTPACMEIntegration` and `TestSMTPACMERecoveryAndNotDue` in `internal/server/smtp_certificates_test.go` drive issuance, renewal, served-certificate replacement, persistent reload and interrupted-renewal recovery against a disposable [Pebble](https://github.com/letsencrypt/pebble) CA. The ordinary race suite exercises SMTP ticket-reuse policy, TLS selection, expiry, readiness, storage failures, cancellation, retries and redirects without a CA.

Start Pebble on loopback with `httpPort` matching an unused backend test port, `profiles.default.validityPeriod` of 12 seconds and `profiles.normal.validityPeriod` of 7776000 seconds (90 days). The tests select these profiles explicitly. Keep actual validation enabled: `PEBBLE_VA_ALWAYS_VALID=0`, `PEBBLE_VA_NOSLEEP=1`, `PEBBLE_AUTHZREUSE=0`. Supply its HTTPS server trust root and the generated issuing root from its management `/roots/0` endpoint as separate PEM files:

```sh
MORGENBLAU_TEST_ACME_CA=https://localhost:14000/dir \
MORGENBLAU_TEST_ACME_HTTP_PORT=18080 \
MORGENBLAU_TEST_ACME_ROOT=/path/to/pebble.minica.pem \
MORGENBLAU_TEST_ACME_CERT_ROOT=/path/to/generated-issuing-root.pem \
  go test -race ./internal/server -run 'TestSMTPACME(Integration|RecoveryAndNotDue)' -count=1 -v
```

For a running isolated app, use the `verify-morgenblau` skill and its newsletter receive/refused-recipient path. `SMTP_ACME_CA` and `SMTP_ACME_CA_ROOT` overrides work only in `APP_ENV=local`; keep test certificates in a disposable run directory, never `/data/certmagic` used by a real deployment. Stop the CA and instance and remove their private assets after verification.

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
```

Use your account's stable DID, not its handle. You still sign in with your handle normally. `ALPHA_ENABLED=true` is already in `fly.toml`; an empty or malformed list then refuses startup. The callback checks the verified DID before setting a cookie or dispatching sync. Existing sessions are checked before resuming, so removing a DID takes effect after the Machine restarts.

The `endpoint=` URL parameter is intentional: setting only `AWS_ENDPOINT_URL_S3` bypasses Litestream's [Tigris-specific detection](https://litestream.io/guides/tigris/). `fly storage create` may set that extra variable, but our explicit URL supplies the correct endpoint.

Back up the production encryption/signing keys separately from SQLite. Restoring the database without `SESSION_STORE_KEYS` cannot recover its encrypted sessions. Do not regenerate keys on each deploy.

```sh
fly secrets import --stage --app "$FLY_APP" < .env.production
```

**Bootstrap is one-time only.** `LITESTREAM_ALLOW_EMPTY_REPLICA=true` permits a first database when the bucket is empty. Remove it after the backup check in step 5. Normally an absent local database must restore successfully before migrations run; inaccessible, empty, or broken backups must not silently become a new empty app.

For the fastest RSS-only first deployment, temporarily set `NEWSLETTER_DOMAIN=""` and `SMTP_LISTEN_ADDR=""` in `fly.toml` and skip SMTP certificates. Complete step 3 and restore those settings before expecting inbound newsletters to work. You can defer the dedicated IPv4 until then.

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

Use a publicly trusted Let's Encrypt certificate for **`mx.app.morgen.blue`**, not a Cloudflare Origin CA certificate. Obtain it with [Certbot's Cloudflare DNS-01 plugin](https://certbot-dns-cloudflare.readthedocs.io/en/stable/); this needs no open challenge port and does not change your MX records.

Run the following on a trusted Debian/Ubuntu host where renewal can run reliably. It can be an existing host; a sleeping laptop is not a reliable renewal host. If you initially issue from your computer, maintain renewal manually until you move this setup to a reliable host. Do not run these commands inside the Fly app's ephemeral filesystem.

```sh
sudo apt-get update
sudo apt-get install certbot python3-certbot-dns-cloudflare
sudo install -d -m 700 /etc/letsencrypt/secrets
sudo install -m 600 /dev/null /etc/letsencrypt/secrets/cloudflare.ini
sudoedit /etc/letsencrypt/secrets/cloudflare.ini
```

In [Cloudflare's API token settings](https://dash.cloudflare.com/profile/api-tokens), create a custom token with **Zone → DNS → Edit**, restricted to **Include → Specific zone → morgen.blue**. Do not use the Global API Key. Put the token in the private INI file, not the repository, shell history, or chat:

```ini
dns_cloudflare_api_token = YOUR_RESTRICTED_CLOUDFLARE_TOKEN
```

Issue the certificate; replace the contact email with a real address you monitor and review Let's Encrypt's terms when prompted:

```sh
sudo certbot certonly --dns-cloudflare \
  --dns-cloudflare-credentials /etc/letsencrypt/secrets/cloudflare.ini \
  --dns-cloudflare-propagation-seconds 60 \
  --cert-name mx.app.morgen.blue -d mx.app.morgen.blue \
  --email you@example.com
```

For unattended uploads, install flyctl on this host and authenticate as in step 1. From the repository checkout, copy the upload script and flyctl into root-owned paths. Skip the second command if `fly` is already installed at `/usr/local/bin/fly`:

```sh
sudo install -D -m 755 scripts/update-fly-smtp-cert.sh /usr/local/lib/morgenblau/update-fly-smtp-cert.sh
sudo install -m 755 "$(command -v fly)" /usr/local/bin/fly
sudo install -m 600 /dev/null /etc/letsencrypt/secrets/morgenblau-fly.token
fly tokens create deploy --app "$FLY_APP" --name smtp-certificate-renewal --expiry 8760h \
  | sudo tee /etc/letsencrypt/secrets/morgenblau-fly.token >/dev/null
```

This [app-scoped deploy token](https://fly.io/docs/security/tokens/) can manage the app, including its secrets; it is not limited to certificate uploads. Keep it private and rotate it before its one-year expiry. Certbot's root timer does not inherit your interactive Fly login or shell variables.

Create the persistent deploy hook below, **replacing the app-name placeholder**. It ignores certificates for other services on the same host:

```sh
sudo install -d -m 755 /etc/letsencrypt/renewal-hooks/deploy
sudo tee /etc/letsencrypt/renewal-hooks/deploy/morgenblau-smtp >/dev/null <<'SH'
#!/bin/sh
set -eu
[ "$RENEWED_LINEAGE" = /etc/letsencrypt/live/mx.app.morgen.blue ] || exit 0
export PATH=/usr/local/bin:/usr/bin:/bin
export FLY_APP=your-globally-unique-app-name
export FLY_API_TOKEN="$(cat /etc/letsencrypt/secrets/morgenblau-fly.token)"
exec /usr/local/lib/morgenblau/update-fly-smtp-cert.sh
SH
sudo chmod 700 /etc/letsencrypt/renewal-hooks/deploy/morgenblau-smtp
```

Upload the first certificate before the newsletter-enabled deployment, then test DNS renewal without uploading a staging certificate:

```sh
sudo env RENEWED_LINEAGE=/etc/letsencrypt/live/mx.app.morgen.blue \
  /etc/letsencrypt/renewal-hooks/deploy/morgenblau-smtp
sudo certbot renew --cert-name mx.app.morgen.blue --dry-run
sudo systemctl enable --now certbot.timer
sudo systemctl list-timers certbot.timer
```

The dry run checks issuance, not the upload hook; the explicit hook invocation checks the Fly upload. The timer runs `certbot renew` and automatically invokes the installed hook after a successful renewal. Check `journalctl -u certbot.service` and monitor failures. If upload fails, fix the cause and rerun the explicit hook; a certificate renewed locally is not necessarily installed on Fly. If you must renew manually, run `sudo certbot renew` regularly on the computer with this setup and confirm the upload succeeds.

The upload script imports base64 PEM secrets without putting their values in command arguments. Production startup validates hostname and expiry. An expired certificate will prevent the app from starting on its next restart. Secret imports restart the Machine; the volume survives.

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
```

Expect successful certificate verification. If your local ISP blocks outbound port 25, test from another network. The product is a receiver, not an outgoing mail relay.

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

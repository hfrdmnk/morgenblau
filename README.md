# Morgenblau

A calm content platform powered by RSS and ATProto. See [SPEC.md](./SPEC.md) for the product vision and [CLAUDE.md](./CLAUDE.md) for stack/conventions.

## Local development

Standard Laravel + Vite setup. With dependencies installed (`composer install && bun install`) and a populated `.env`, run:

```sh
php artisan solo
```

…which starts `Serve` (PHP on `:8000`), `Tunnel` (Cloudflare named tunnel), `Vite` (bun dev), and `Queue`.

### Dev OAuth tunnel — one-time setup

ATProto OAuth requires Bluesky's authorization server to fetch your `/oauth-client-metadata.json` over public HTTPS. Without that, the consent screen falls back to identity-only and the app can't request granular `repo:app.skyreader.*` scopes. We expose `127.0.0.1:8000` to the internet via a Cloudflare named tunnel pinned to `<subdomain>.morgen.blue`.

1. **Install cloudflared:**

    ```sh
    brew install cloudflared
    ```

2. **Authenticate with Cloudflare** (opens a browser; pick the `morgen.blue` zone):

    ```sh
    cloudflared tunnel login
    ```

3. **Create a named tunnel** (writes credentials to `~/.cloudflared/<UUID>.json`):

    ```sh
    cloudflared tunnel create morgenblau-<subdomain>
    ```

4. **Route the subdomain** (creates a `CNAME` in Cloudflare DNS, stable forever):

    ```sh
    cloudflared tunnel route dns morgenblau-<subdomain> <subdomain>.morgen.blue
    ```

5. **Write `~/.cloudflared/config.yml`** (replace `<UUID>` with the value from step 3):

    ```yaml
    tunnel: morgenblau-<subdomain>
    credentials-file: /Users/<your-mac-user>/.cloudflared/<UUID>.json

    ingress:
        - hostname: <subdomain>.morgen.blue
          service: http://localhost:8000
        - service: http_status:404
    ```

6. **Set `.env`**:

    ```dotenv
    APP_URL=https://<subdomain>.morgen.blue
    BLUESKY_CLIENT_ID=https://<subdomain>.morgen.blue/oauth-client-metadata.json
    BLUESKY_REDIRECT=https://<subdomain>.morgen.blue/oauth/callback

    # Required: the AS validates client_id by fetching /oauth-client-metadata.json
    # synchronously during PAR. With the default 1-worker `php artisan serve`,
    # the PAR request and the AS's metadata fetch deadlock and PHP times out at 30s.
    PHP_CLI_SERVER_WORKERS=4
    ```

7. **Run** `php artisan solo`. The `Tunnel` process picks up `~/.cloudflared/config.yml` and serves the subdomain. Visit `https://<subdomain>.morgen.blue/login` to test the OAuth flow.

The `Tunnel` command in `config/solo.php` deliberately takes no arguments — it reads `~/.cloudflared/config.yml`, which is per-machine. The committed `solo.php` stays dev-agnostic.

### Verifying the OAuth metadata document

```sh
curl -s https://<subdomain>.morgen.blue/oauth-client-metadata.json | jq
curl -s https://<subdomain>.morgen.blue/oauth-jwks.json | jq '.keys[0] | keys'
```

The metadata `scope` should list all four `repo:app.skyreader.*` collections. The JWKS must not expose a `d` field (that would be a private-key leak).

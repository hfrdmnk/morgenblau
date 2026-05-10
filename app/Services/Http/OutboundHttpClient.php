<?php

namespace App\Services\Http;

use App\Exceptions\UnsafeUrlException;
use Closure;
use GuzzleHttp\Psr7\LimitStream;
use GuzzleHttp\Psr7\Response as GuzzleResponse;
use GuzzleHttp\Psr7\Stream;
use GuzzleHttp\Psr7\Utils as Psr7Utils;
use Illuminate\Http\Client\PendingRequest;
use Illuminate\Http\Client\Response;
use Illuminate\Support\Facades\Http;
use Psr\Http\Message\RequestInterface;
use Psr\Http\Message\ResponseInterface;
use Psr\Http\Message\UriInterface;

/**
 * SSRF-aware outbound HTTP client. Used by feed adapters to fetch user-supplied
 * URLs safely: rejects loopback, RFC1918, link-local, IPv6 ULA + IPv4-mapped
 * IPv6, non-http(s) schemes; pins validated IPs into cURL on the initial fetch
 * so the kernel cannot re-resolve to a private address, and re-validates on
 * every redirect.
 *
 * Trusted destinations (e.g. itunes.apple.com) bypass the IP guard via
 * getTrusted() but still get timeouts and the body cap.
 */
class OutboundHttpClient
{
    private const MAX_RESPONSE_BYTES = 5 * 1024 * 1024;

    private const CONNECT_TIMEOUT = 3;

    private const REQUEST_TIMEOUT = 8;

    private const MAX_REDIRECTS = 5;

    public function __construct(private readonly DnsResolver $dns) {}

    /**
     * Fetch a URL provided by the user. Validates scheme, host, and resolved
     * IPs against private/loopback ranges; pins those IPs into cURL; re-validates
     * each redirect target before Guzzle dispatches the next hop.
     *
     * @param  array<string, string>  $headers
     */
    public function getUserUrl(string $url, array $headers = []): Response
    {
        return $this->sendUserUrl('GET', $url, $headers);
    }

    /**
     * Generalised SSRF-guarded send. Underpins getUserUrl and any caller that
     * needs a non-GET method (e.g. PSR-18 adapter performing HEAD probes for
     * favicon discovery).
     *
     * @param  array<string, string>  $headers
     */
    public function sendUserUrl(string $method, string $url, array $headers = []): Response
    {
        $ips = $this->assertUserUrlIsSafe($url);

        $parts = parse_url($url);
        $host = strtolower(trim((string) $parts['host'], '[]'));
        $scheme = strtolower((string) $parts['scheme']);
        $port = $parts['port'] ?? ($scheme === 'https' ? 443 : 80);

        return $this->base($headers)
            ->withOptions([
                'curl' => [
                    CURLOPT_RESOLVE => ["{$host}:{$port}:".implode(',', $ips)],
                ],
                'allow_redirects' => [
                    'max' => self::MAX_REDIRECTS,
                    'protocols' => ['http', 'https'],
                    'strict' => true,
                    // Cross-host redirects re-resolve via cURL since CURLOPT_RESOLVE
                    // only pins the initial host. assertSafeRedirectTarget validates
                    // the new host's IPs before Guzzle dispatches; a TOCTOU window
                    // remains between that check and the connect — accepted given
                    // the 50/min rate limit and 5MB body cap.
                    'on_redirect' => function (RequestInterface $req, ResponseInterface $res, UriInterface $next): void {
                        $this->assertSafeRedirectTarget((string) $next);
                    },
                ],
            ])
            ->send($method, $url);
    }

    /**
     * Public seam for redirect re-validation. The on_redirect Closure delegates
     * here so tests can drive it directly (Http::fake bypasses Guzzle's redirect
     * middleware, so the Closure cannot be exercised through faked requests).
     */
    public function assertSafeRedirectTarget(string $url): void
    {
        $this->assertUserUrlIsSafe($url);
    }

    /**
     * Fetch from a known-safe, hard-coded host. Skips the IP guard but applies
     * timeouts, scheme whitelist, redirect cap, and body cap.
     *
     * @param  array<string, mixed>  $query
     * @param  array<string, string>  $headers
     */
    public function getTrusted(string $url, array $query = [], array $headers = []): Response
    {
        return $this->base($headers)->get($url, $query);
    }

    /**
     * @param  array<string, string>  $headers
     */
    private function base(array $headers): PendingRequest
    {
        return Http::withHeaders($headers)
            ->connectTimeout(self::CONNECT_TIMEOUT)
            ->timeout(self::REQUEST_TIMEOUT)
            ->withOptions([
                'allow_redirects' => [
                    'max' => self::MAX_REDIRECTS,
                    'protocols' => ['http', 'https'],
                    'strict' => true,
                ],
            ])
            ->withMiddleware($this->bodySizeMiddleware());
    }

    /**
     * @return non-empty-list<string> validated IPs, returned so callers can
     *                                pin them via CURLOPT_RESOLVE without
     *                                resolving twice.
     */
    private function assertUserUrlIsSafe(string $url): array
    {
        $parts = parse_url($url);
        if ($parts === false || ! isset($parts['scheme'], $parts['host'])) {
            throw new UnsafeUrlException("Malformed URL: {$url}.");
        }

        $scheme = strtolower((string) $parts['scheme']);
        if (! in_array($scheme, ['http', 'https'], true)) {
            throw new UnsafeUrlException("Disallowed scheme '{$scheme}' for {$url}.");
        }

        $host = strtolower((string) $parts['host']);
        // Strip IPv6 literal brackets if present.
        $host = trim($host, '[]');
        if ($host === '' || $host === 'localhost' || str_ends_with($host, '.localhost')) {
            throw new UnsafeUrlException("Loopback host blocked: {$url}.");
        }

        $ips = $this->dns->resolve($host);
        if ($ips === []) {
            throw new UnsafeUrlException("Could not resolve {$host}.");
        }

        foreach ($ips as $ip) {
            if (! $this->ipIsPublic($ip)) {
                throw new UnsafeUrlException("Resolved {$host} to private IP {$ip}.");
            }
        }

        return $ips;
    }

    /**
     * FILTER_FLAG_NO_PRIV_RANGE rejects RFC1918 (10/8, 172.16/12, 192.168/16)
     * and IPv6 ULA (fc00::/7). FILTER_FLAG_NO_RES_RANGE rejects 0.0.0.0/8,
     * 127/8, 169.254/16 (which covers AWS IMDS at 169.254.169.254), ::1,
     * fe80::/10, etc. IPv4-mapped IPv6 (::ffff:0:0/96) is NOT covered by
     * those flags, so we extract the embedded IPv4 and re-check.
     */
    private function ipIsPublic(string $ip): bool
    {
        if (filter_var($ip, FILTER_VALIDATE_IP, FILTER_FLAG_IPV6) !== false) {
            $mapped = $this->extractMappedIPv4($ip);
            if ($mapped !== null) {
                return (bool) filter_var(
                    $mapped,
                    FILTER_VALIDATE_IP,
                    FILTER_FLAG_IPV4 | FILTER_FLAG_NO_PRIV_RANGE | FILTER_FLAG_NO_RES_RANGE,
                );
            }
        }

        return (bool) filter_var(
            $ip,
            FILTER_VALIDATE_IP,
            FILTER_FLAG_IPV4 | FILTER_FLAG_IPV6 | FILTER_FLAG_NO_PRIV_RANGE | FILTER_FLAG_NO_RES_RANGE,
        );
    }

    /**
     * Returns the embedded IPv4 string when $ipv6 falls in ::ffff:0:0/96.
     * Handles both ::ffff:127.0.0.1 (mixed) and ::ffff:7f00:1 (pure hex).
     */
    private function extractMappedIPv4(string $ipv6): ?string
    {
        $packed = @inet_pton($ipv6);
        if ($packed === false || strlen($packed) !== 16) {
            return null;
        }

        if (substr($packed, 0, 10) !== str_repeat("\0", 10) || substr($packed, 10, 2) !== "\xff\xff") {
            return null;
        }

        $ipv4 = @inet_ntop("\0\0\0\0".substr($packed, 12));

        return $ipv4 === false ? null : $ipv4;
    }

    /**
     * Guzzle middleware: caps the response body to MAX_RESPONSE_BYTES by
     * wrapping it in a LimitStream. Protects against memory blow-ups when
     * fetching arbitrary user URLs.
     */
    private function bodySizeMiddleware(): Closure
    {
        return function (Closure $handler): Closure {
            return function (RequestInterface $request, array $options) use ($handler) {
                return $handler($request, $options)->then(function (ResponseInterface $response): ResponseInterface {
                    $body = $response->getBody();
                    $stream = $body instanceof Stream ? $body : Psr7Utils::streamFor($body);
                    $limited = new LimitStream($stream, self::MAX_RESPONSE_BYTES);

                    return new GuzzleResponse(
                        $response->getStatusCode(),
                        $response->getHeaders(),
                        $limited,
                        $response->getProtocolVersion(),
                        $response->getReasonPhrase(),
                    );
                });
            };
        };
    }
}

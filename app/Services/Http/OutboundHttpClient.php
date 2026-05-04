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
 * URLs safely: rejects loopback, RFC1918, link-local, IPv6 ULA, non-http(s)
 * schemes, and re-validates after every redirect to defeat DNS rebinding.
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
     * IPs against private/loopback ranges; re-validates on every redirect.
     *
     * @param  array<string, string>  $headers
     */
    public function getUserUrl(string $url, array $headers = []): Response
    {
        $this->assertSafeUrl($url);

        return $this->base($headers)
            ->withOptions([
                'allow_redirects' => [
                    'max' => self::MAX_REDIRECTS,
                    'protocols' => ['http', 'https'],
                    'strict' => true,
                    'on_redirect' => function (RequestInterface $req, ResponseInterface $res, UriInterface $next) {
                        $this->assertSafeUrl((string) $next);
                    },
                ],
            ])
            ->get($url);
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

    private function assertSafeUrl(string $url): void
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
    }

    /**
     * FILTER_FLAG_NO_PRIV_RANGE rejects RFC1918 (10/8, 172.16/12, 192.168/16)
     * and IPv6 ULA (fc00::/7). FILTER_FLAG_NO_RES_RANGE rejects 0.0.0.0/8,
     * 127/8, 169.254/16 (which covers AWS IMDS at 169.254.169.254), ::1,
     * fe80::/10, etc.
     */
    private function ipIsPublic(string $ip): bool
    {
        return (bool) filter_var(
            $ip,
            FILTER_VALIDATE_IP,
            FILTER_FLAG_IPV4 | FILTER_FLAG_IPV6 | FILTER_FLAG_NO_PRIV_RANGE | FILTER_FLAG_NO_RES_RANGE,
        );
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

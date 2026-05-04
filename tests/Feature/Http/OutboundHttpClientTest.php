<?php

use App\Exceptions\UnsafeUrlException;
use App\Services\Http\DnsResolver;
use App\Services\Http\OutboundHttpClient;
use Illuminate\Support\Facades\Http;
use Tests\Doubles\FakeDnsResolver;

beforeEach(function () {
    Http::preventStrayRequests();
});

function bindDns(array $map): void
{
    app()->bind(DnsResolver::class, fn () => new FakeDnsResolver($map));
}

test('rejects non-http(s) schemes', function () {
    bindDns([]);

    expect(fn () => app(OutboundHttpClient::class)->getUserUrl('javascript:alert(1)'))
        ->toThrow(UnsafeUrlException::class)
        ->and(fn () => app(OutboundHttpClient::class)->getUserUrl('file:///etc/passwd'))
        ->toThrow(UnsafeUrlException::class);
});

test('rejects loopback hostnames literally', function () {
    bindDns([]);

    expect(fn () => app(OutboundHttpClient::class)->getUserUrl('http://localhost/x'))
        ->toThrow(UnsafeUrlException::class, 'Loopback')
        ->and(fn () => app(OutboundHttpClient::class)->getUserUrl('http://foo.localhost/x'))
        ->toThrow(UnsafeUrlException::class, 'Loopback');
});

test('rejects hosts that resolve to 127.0.0.1', function () {
    bindDns(['evil.example' => ['127.0.0.1']]);

    expect(fn () => app(OutboundHttpClient::class)->getUserUrl('http://evil.example/'))
        ->toThrow(UnsafeUrlException::class, 'private IP');
});

test('rejects hosts that resolve to RFC1918 ranges', function () {
    bindDns(['internal.example' => ['10.0.0.5']]);

    expect(fn () => app(OutboundHttpClient::class)->getUserUrl('http://internal.example/'))
        ->toThrow(UnsafeUrlException::class, 'private IP');
});

test('rejects the AWS instance metadata link-local address', function () {
    bindDns(['rebound.example' => ['169.254.169.254']]);

    expect(fn () => app(OutboundHttpClient::class)->getUserUrl('http://rebound.example/'))
        ->toThrow(UnsafeUrlException::class, 'private IP');
});

test('rejects IPv6 loopback and ULA', function () {
    // Hosts are IP literals — FakeDnsResolver short-circuits to return them.
    bindDns([]);

    expect(fn () => app(OutboundHttpClient::class)->getUserUrl('http://[::1]/x'))
        ->toThrow(UnsafeUrlException::class, 'private IP')
        ->and(fn () => app(OutboundHttpClient::class)->getUserUrl('http://[fc00::1]/x'))
        ->toThrow(UnsafeUrlException::class, 'private IP');
});

test('rejects IPv4-mapped IPv6 forms that wrap private IPv4', function () {
    // FILTER_FLAG_NO_PRIV_RANGE | FILTER_FLAG_NO_RES_RANGE only flag native
    // IPv6 ranges; ::ffff:0:0/96 mapped IPv4 must be unwrapped + re-checked.
    bindDns([]);

    expect(fn () => app(OutboundHttpClient::class)->getUserUrl('http://[::ffff:127.0.0.1]/x'))
        ->toThrow(UnsafeUrlException::class, 'private IP')
        ->and(fn () => app(OutboundHttpClient::class)->getUserUrl('http://[::ffff:7f00:1]/x'))
        ->toThrow(UnsafeUrlException::class, 'private IP')
        ->and(fn () => app(OutboundHttpClient::class)->getUserUrl('http://[::ffff:169.254.169.254]/x'))
        ->toThrow(UnsafeUrlException::class, 'private IP');
});

test('rejects hosts whose DNS A record points at an IPv4-mapped IPv6 loopback', function () {
    bindDns(['mapped.example' => ['::ffff:127.0.0.1']]);

    expect(fn () => app(OutboundHttpClient::class)->getUserUrl('http://mapped.example/'))
        ->toThrow(UnsafeUrlException::class, 'private IP');
});

test('assertSafeRedirectTarget rejects a redirect to a loopback host', function () {
    // Http::fake bypasses Guzzle's redirect middleware, so on_redirect can't be
    // exercised through fakes — the public seam is what real redirects call.
    bindDns(['rebound.example' => ['127.0.0.1']]);

    expect(fn () => app(OutboundHttpClient::class)->assertSafeRedirectTarget('http://rebound.example/'))
        ->toThrow(UnsafeUrlException::class, 'private IP');
});

test('assertSafeRedirectTarget rejects a non-http(s) downgrade', function () {
    bindDns([]);

    expect(fn () => app(OutboundHttpClient::class)->assertSafeRedirectTarget('ftp://example.com/'))
        ->toThrow(UnsafeUrlException::class, 'Disallowed scheme');
});

test('assertSafeRedirectTarget rejects an IPv4-mapped IPv6 redirect target', function () {
    bindDns(['mapped-redirect.example' => ['::ffff:127.0.0.1']]);

    expect(fn () => app(OutboundHttpClient::class)->assertSafeRedirectTarget('http://mapped-redirect.example/'))
        ->toThrow(UnsafeUrlException::class, 'private IP');
});

test('rejects unresolvable hosts', function () {
    bindDns([]);

    expect(fn () => app(OutboundHttpClient::class)->getUserUrl('http://no-such-host.test/'))
        ->toThrow(UnsafeUrlException::class, 'Could not resolve');
});

test('allows public hosts', function () {
    bindDns(['example.com' => ['93.184.216.34']]);

    Http::fake([
        '*example.com*' => Http::response('ok', 200),
    ]);

    $response = app(OutboundHttpClient::class)->getUserUrl('https://example.com');

    expect($response->ok())->toBeTrue();
});

test('getTrusted skips the IP guard but still timeouts via base()', function () {
    bindDns([]); // empty — would fail IP guard, but getTrusted bypasses it

    Http::fake([
        'itunes.apple.com/lookup*' => Http::response(['ok' => true], 200),
    ]);

    $response = app(OutboundHttpClient::class)->getTrusted(
        'https://itunes.apple.com/lookup',
        ['id' => 123],
    );

    expect($response->ok())->toBeTrue();
});

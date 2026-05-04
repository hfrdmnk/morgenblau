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

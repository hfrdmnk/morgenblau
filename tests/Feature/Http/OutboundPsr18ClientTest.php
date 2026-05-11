<?php

use App\Services\Http\OutboundHttpClient;
use App\Services\Http\OutboundPsr18Client;
use GuzzleHttp\Psr7\Request;
use GuzzleHttp\Psr7\Response as GuzzleResponse;
use Illuminate\Http\Client\Response;
use Psr\Http\Client\NetworkExceptionInterface;

test('sendRequest forwards method, url, headers to OutboundHttpClient and returns a PSR-7 response', function () {
    $guzzleResponse = new GuzzleResponse(
        200,
        ['Content-Type' => ['image/png'], 'X-Trace' => ['abc']],
        'binary-bytes',
    );
    $laravelResponse = new Response($guzzleResponse);

    $http = Mockery::mock(OutboundHttpClient::class);
    $http->shouldReceive('sendUserUrl')
        ->once()
        ->with('HEAD', 'https://example.com/favicon.ico', ['Host' => 'example.com', 'Accept' => 'image/*'])
        ->andReturn($laravelResponse);

    $client = new OutboundPsr18Client($http);
    $request = new Request('HEAD', 'https://example.com/favicon.ico', ['Accept' => 'image/*']);

    $response = $client->sendRequest($request);

    expect($response->getStatusCode())->toBe(200);
    expect($response->getHeaderLine('Content-Type'))->toBe('image/png');
    expect($response->getHeaderLine('X-Trace'))->toBe('abc');
    expect((string) $response->getBody())->toBe('binary-bytes');
});

test('sendRequest collapses multi-value headers when forwarding', function () {
    $guzzleResponse = new GuzzleResponse(204);
    $laravelResponse = new Response($guzzleResponse);

    $http = Mockery::mock(OutboundHttpClient::class);
    $http->shouldReceive('sendUserUrl')
        ->once()
        ->with('GET', 'https://example.com/', ['Host' => 'example.com', 'Accept' => 'text/html, application/xhtml+xml'])
        ->andReturn($laravelResponse);

    $client = new OutboundPsr18Client($http);
    $request = (new Request('GET', 'https://example.com/'))
        ->withHeader('Accept', ['text/html', 'application/xhtml+xml']);

    $client->sendRequest($request);
});

test('sendRequest wraps a thrown OutboundHttpClient error as NetworkExceptionInterface', function () {
    $http = Mockery::mock(OutboundHttpClient::class);
    $http->shouldReceive('sendUserUrl')->andThrow(new RuntimeException('SSRF refused'));

    $client = new OutboundPsr18Client($http);
    $request = new Request('GET', 'https://example.com/icon.svg');

    try {
        $client->sendRequest($request);
        $this->fail('expected NetworkExceptionInterface');
    } catch (NetworkExceptionInterface $e) {
        expect($e->getRequest())->toBe($request);
        expect($e->getPrevious()?->getMessage())->toBe('SSRF refused');
    }
});

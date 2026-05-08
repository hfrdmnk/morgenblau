<?php

namespace App\Services\Feeds;

use App\Services\Http\OutboundHttpClient;
use DateTime;
use FeedIo\Adapter\ClientInterface;
use FeedIo\Adapter\Http\Response;
use FeedIo\Adapter\NotFoundException;
use FeedIo\Adapter\ResponseInterface;
use FeedIo\Adapter\ServerErrorException;

/**
 * Bridges feed-io's HTTP client contract onto OutboundHttpClient so feed
 * fetches inherit our SSRF guard, redirect re-validation, and body cap.
 * UnsafeUrlException is intentionally not caught — FeedFetcher wraps any
 * Throwable in FeedFetchException, and the SSRF reason is more useful at
 * that layer than feed-io's generic ServerErrorException.
 */
class OutboundFeedClient implements ClientInterface, ConditionalFeedClient
{
    public function __construct(private readonly OutboundHttpClient $http) {}

    public function getResponse(string $url, ?DateTime $modifiedSince = null): ResponseInterface
    {
        $lastModified = $modifiedSince?->format(DateTime::RFC2822);

        return $this->fetchConditional($url, etag: null, lastModified: $lastModified);
    }

    /**
     * Fetch a feed with optional conditional-GET headers. Returns the feed-io
     * response so callers (FeedFetcher) can branch on 304 vs 200 before parsing.
     * 304 responses bypass the >=400 throw path here — they are normal, not errors.
     */
    public function fetchConditional(string $url, ?string $etag, ?string $lastModified): ResponseInterface
    {
        $headers = [];
        if ($etag !== null && $etag !== '') {
            $headers['If-None-Match'] = $etag;
        }
        if ($lastModified !== null && $lastModified !== '') {
            $headers['If-Modified-Since'] = $lastModified;
        }

        $start = microtime(true);
        $response = $this->http->getUserUrl($url, $headers);
        $duration = microtime(true) - $start;

        $status = $response->status();
        $psr = $response->toPsrResponse();

        if ($status === 304) {
            return new Response($psr, $duration);
        }

        if ($status === 404) {
            throw new NotFoundException('not found', $duration);
        }

        if ($status >= 400) {
            throw new ServerErrorException($psr, $duration);
        }

        return new Response($psr, $duration);
    }
}

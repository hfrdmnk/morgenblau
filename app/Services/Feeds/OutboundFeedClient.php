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
class OutboundFeedClient implements ClientInterface
{
    public function __construct(private readonly OutboundHttpClient $http) {}

    public function getResponse(string $url, ?DateTime $modifiedSince = null): ResponseInterface
    {
        $headers = $modifiedSince !== null
            ? ['If-Modified-Since' => $modifiedSince->format(DateTime::RFC2822)]
            : [];

        $start = microtime(true);
        $response = $this->http->getUserUrl($url, $headers);
        $duration = microtime(true) - $start;

        $status = $response->status();

        if ($status === 404) {
            throw new NotFoundException('not found', $duration);
        }

        $psr = $response->toPsrResponse();

        if ($status >= 400) {
            throw new ServerErrorException($psr, $duration);
        }

        return new Response($psr, $duration);
    }
}

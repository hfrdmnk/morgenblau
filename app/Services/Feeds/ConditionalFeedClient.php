<?php

namespace App\Services\Feeds;

use FeedIo\Adapter\ResponseInterface;

interface ConditionalFeedClient
{
    public function fetchConditional(string $url, ?string $etag, ?string $lastModified): ResponseInterface;
}

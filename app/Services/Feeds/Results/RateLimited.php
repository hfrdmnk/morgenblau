<?php

namespace App\Services\Feeds\Results;

final class RateLimited extends FetchedFeedResult
{
    public function __construct(
        public readonly int $retryAfterSeconds,
    ) {}
}

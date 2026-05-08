<?php

namespace App\Services\Feeds\Results;

final class NotModified extends FetchedFeedResult
{
    public function __construct(
        public readonly ?string $etag,
        public readonly ?string $lastModified,
    ) {}
}

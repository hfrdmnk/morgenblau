<?php

namespace App\Services\Feeds\Results;

use App\Data\Feeds\FetchedEntryData;

final class Modified extends FetchedFeedResult
{
    /**
     * @param  list<FetchedEntryData>  $entries
     */
    public function __construct(
        public readonly array $entries,
        public readonly ?string $etag,
        public readonly ?string $lastModified,
    ) {}
}

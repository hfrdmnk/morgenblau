<?php

namespace App\Services\Feeds;

use App\Data\Feeds\ResolvedFeedData;

interface FeedAdapter
{
    /**
     * Whether this adapter wants to handle the URL. The resolver picks the
     * first adapter whose claims() returns true and dispatches resolve() to it.
     */
    public function claims(string $url): bool;

    /**
     * Resolve the URL into one or more candidate feeds. Caller guarantees
     * claims($url) returned true. Throws UnresolvableFeedException on failure.
     *
     * @return non-empty-list<ResolvedFeedData>
     */
    public function resolve(string $url): array;
}

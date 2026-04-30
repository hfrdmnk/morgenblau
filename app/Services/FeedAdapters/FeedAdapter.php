<?php

namespace App\Services\FeedAdapters;

interface FeedAdapter
{
    /**
     * Attempt to resolve the URL into a ResolvedFeed.
     * Return null if this adapter cannot handle the URL — the resolver will try the next one.
     */
    public function tryResolve(string $url): ?ResolvedFeed;
}

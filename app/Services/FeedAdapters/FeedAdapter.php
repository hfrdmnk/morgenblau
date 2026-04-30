<?php

namespace App\Services\FeedAdapters;

interface FeedAdapter
{
    /**
     * Attempt to resolve the URL into one or more candidate feeds.
     * Return [] if this adapter cannot handle the URL — the resolver will try the next one.
     *
     * @return list<ResolvedFeed>
     */
    public function tryResolve(string $url): array;
}

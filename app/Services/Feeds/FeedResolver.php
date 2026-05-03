<?php

namespace App\Services\Feeds;

use App\Data\Feeds\ResolvedFeedData;
use App\Services\Feeds\Exceptions\UnresolvableFeedException;

class FeedResolver
{
    /**
     * @param  iterable<FeedAdapter>  $adapters
     */
    public function __construct(private readonly iterable $adapters) {}

    /**
     * @return non-empty-list<ResolvedFeedData>
     */
    public function resolve(string $url): array
    {
        foreach ($this->adapters as $adapter) {
            $candidates = $adapter->tryResolve($url);

            if ($candidates !== []) {
                return $candidates;
            }
        }

        throw new UnresolvableFeedException("Couldn't find a feed for {$url}.");
    }
}

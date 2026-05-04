<?php

namespace App\Services\Feeds;

use App\Data\Feeds\ResolvedFeedData;
use App\Exceptions\UnresolvableFeedException;

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
            if ($adapter->claims($url)) {
                return $adapter->resolve($url);
            }
        }

        throw new UnresolvableFeedException("Couldn't find a feed for {$url}.");
    }
}

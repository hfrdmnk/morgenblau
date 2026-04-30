<?php

namespace App\Services\FeedAdapters;

use App\Services\FeedAdapters\Exceptions\UnresolvableFeedException;

class FeedResolver
{
    /**
     * @param  iterable<FeedAdapter>  $adapters
     */
    public function __construct(private readonly iterable $adapters) {}

    public function resolve(string $url): ResolvedFeed
    {
        foreach ($this->adapters as $adapter) {
            $resolved = $adapter->tryResolve($url);

            if ($resolved !== null) {
                return $resolved;
            }
        }

        throw new UnresolvableFeedException("Couldn't find a feed for {$url}.");
    }
}

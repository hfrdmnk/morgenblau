<?php

namespace App\Services\FeedAdapters;

use App\Services\FeedAdapters\Exceptions\UnresolvableFeedException;

class FeedResolver
{
    /**
     * @param  iterable<FeedAdapter>  $adapters
     */
    public function __construct(private readonly iterable $adapters) {}

    /**
     * @return non-empty-list<ResolvedFeed>
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

<?php

namespace App\Services\FeedAdapters;

final class ResolvedFeed
{
    public function __construct(
        public readonly string $feedUrl,
        public readonly ?string $title,
        public readonly ?string $siteUrl,
        public readonly string $category,
        public readonly string $sourceType = 'rss',
    ) {}
}

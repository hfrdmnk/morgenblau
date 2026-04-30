<?php

namespace App\Services;

final class ChosenFeed
{
    public function __construct(
        public readonly string $feedUrl,
        public readonly ?string $title,
        public readonly ?string $siteUrl,
        public readonly string $sourceType,
    ) {}
}

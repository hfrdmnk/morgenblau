<?php

namespace App\Services\Feeds\Results;

use Throwable;

final class Failed extends FetchedFeedResult
{
    public function __construct(
        public readonly Throwable $cause,
    ) {}
}

<?php

namespace App\Contracts\Feeds;

use App\Models\Feed;

interface FaviconDiscovererInterface
{
    public function discover(Feed $feed): void;
}

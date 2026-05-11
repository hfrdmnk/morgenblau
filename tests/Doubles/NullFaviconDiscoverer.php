<?php

namespace Tests\Doubles;

use App\Contracts\Feeds\FaviconDiscovererInterface;
use App\Models\Feed;

/**
 * Test double bound in feed-refresh tests so RefreshFeedJob doesn't try to
 * make real HTTP calls for favicon discovery during unrelated assertions.
 * Tests that *do* care about discovery (FaviconDiscovererTest) leave the real
 * binding in place.
 */
class NullFaviconDiscoverer implements FaviconDiscovererInterface
{
    public function discover(Feed $feed): void {}
}

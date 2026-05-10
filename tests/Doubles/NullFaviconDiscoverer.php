<?php

namespace Tests\Doubles;

use App\Models\Feed;
use App\Services\Feeds\FaviconDiscoverer;

/**
 * Test double bound in feed-refresh tests so RefreshFeedJob doesn't try to
 * make real HTTP calls for favicon discovery during unrelated assertions.
 * Tests that *do* care about discovery (FaviconDiscovererTest) leave the real
 * binding in place.
 */
class NullFaviconDiscoverer extends FaviconDiscoverer
{
    public function __construct() {}

    public function discover(Feed $feed): void {}
}

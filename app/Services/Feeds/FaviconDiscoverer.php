<?php

namespace App\Services\Feeds;

use App\Models\Feed;
use Carbon\CarbonImmutable;
use FeedIo\FaviconIo\FaviconDiscovery;
use Illuminate\Support\Facades\Date;
use Psr\Log\LoggerInterface;
use Throwable;

/**
 * Resolves and persists a feed's favicon URL via php-feed-io/favicon-io.
 * Discovery uses OutboundPsr18Client, so SSRF/redirect/body-cap guards apply.
 *
 * Re-checks every RECHECK_DAYS — favicons rarely change, and we don't want
 * to issue 1-3 extra HTTP calls per feed on every refresh tick.
 */
class FaviconDiscoverer
{
    private const RECHECK_DAYS = 30;

    public function __construct(
        private readonly FaviconDiscovery $discovery,
        private readonly LoggerInterface $logger,
    ) {}

    public function discover(Feed $feed): void
    {
        if (! $this->isStale($feed)) {
            return;
        }

        $base = $this->baseUrl($feed);
        if ($base === null) {
            return;
        }

        try {
            $url = $this->discovery->discover($base);
        } catch (Throwable $e) {
            $this->logger->info('favicon discovery failed', [
                'feed_id' => $feed->id,
                'base_url' => $base,
                'error' => $e->getMessage(),
            ]);
            $url = null;
        }

        $feed->forceFill([
            'favicon_url' => $url,
            'favicon_checked_at' => Date::now(),
        ])->save();
    }

    private function isStale(Feed $feed): bool
    {
        $checked = $feed->favicon_checked_at;
        if (! $checked instanceof CarbonImmutable) {
            return true;
        }

        return $checked->lessThan(Date::now()->subDays(self::RECHECK_DAYS));
    }

    /**
     * Discovery targets the website root; the package follows redirects and
     * normalises trailing slashes itself. Prefer site_url if known, fall
     * back to the host of feed_url.
     */
    private function baseUrl(Feed $feed): ?string
    {
        foreach ([$feed->site_url, $feed->feed_url] as $candidate) {
            if (! is_string($candidate) || $candidate === '') {
                continue;
            }
            $parts = parse_url($candidate);
            if ($parts === false || ! isset($parts['scheme'], $parts['host'])) {
                continue;
            }

            return $parts['scheme'].'://'.$parts['host'];
        }

        return null;
    }
}

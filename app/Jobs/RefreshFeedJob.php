<?php

namespace App\Jobs;

use App\Models\Feed;
use App\Services\Feeds\FeedEntryUpserter;
use App\Services\Feeds\FeedFetcher;
use App\Services\Feeds\Processors\ProcessorPipeline;
use App\Services\Feeds\Results\Failed;
use App\Services\Feeds\Results\Gone;
use App\Services\Feeds\Results\Modified;
use App\Services\Feeds\Results\NotModified;
use App\Services\Feeds\Results\RateLimited;
use Illuminate\Contracts\Queue\ShouldBeUnique;
use Illuminate\Contracts\Queue\ShouldQueue;
use Illuminate\Foundation\Queue\Queueable;
use Throwable;

class RefreshFeedJob implements ShouldBeUnique, ShouldQueue
{
    use Queueable;

    public int $tries = 1;

    public int $uniqueFor = 300;

    public function __construct(public readonly int $feedId) {}

    public function uniqueId(): string
    {
        return (string) $this->feedId;
    }

    public function handle(FeedFetcher $fetcher, FeedEntryUpserter $upserter, ProcessorPipeline $pipeline): void
    {
        $feed = Feed::query()->find($this->feedId);

        if ($feed === null) {
            return;
        }

        $result = $fetcher->fetch($feed->feed_url, $feed->etag_header, $feed->last_modified_header);

        match (true) {
            $result instanceof Modified => $this->onModified($feed, $upserter, $pipeline, $result),
            $result instanceof NotModified => $feed->markNotModified(),
            $result instanceof Gone => $feed->markGone(),
            $result instanceof RateLimited => $feed->markRateLimited($result->retryAfterSeconds),
            $result instanceof Failed => $feed->markFailed($result->cause),
        };
    }

    public function failed(Throwable $e): void
    {
        try {
            $feed = Feed::query()->find($this->feedId);
        } catch (Throwable) {
            return;
        }

        $feed?->markFailed($e);
    }

    private function onModified(Feed $feed, FeedEntryUpserter $upserter, ProcessorPipeline $pipeline, Modified $result): void
    {
        $upserter->upsert($feed, $pipeline->processBatch($result->entries, $feed));
        $feed->markFetched($result->etag, $result->lastModified);
    }
}

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
use Illuminate\Support\Facades\Date;
use Illuminate\Support\Str;
use Throwable;

class RefreshFeedJob implements ShouldBeUnique, ShouldQueue
{
    use Queueable;

    public int $tries = 1;

    public int $uniqueFor = 300;

    private const REFRESH_INTERVAL_MINUTES = 30;

    private const MUTE_THRESHOLD = 20;

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
            $result instanceof NotModified => $this->onNotModified($feed, $result),
            $result instanceof Gone => $this->onGone($feed),
            $result instanceof RateLimited => $this->onRateLimited($feed, $result),
            $result instanceof Failed => $this->onFailed($feed, $result->cause),
        };
    }

    public function failed(Throwable $e): void
    {
        try {
            $feed = Feed::query()->find($this->feedId);
        } catch (Throwable) {
            return;
        }

        if ($feed === null) {
            return;
        }

        $this->onFailed($feed, $e);
    }

    private function onModified(Feed $feed, FeedEntryUpserter $upserter, ProcessorPipeline $pipeline, Modified $result): void
    {
        $processed = $pipeline->processBatch($result->entries, $feed);
        $upserter->upsert($feed, $processed);

        $feed->forceFill([
            'last_fetched_at' => Date::now(),
            'last_dispatched_at' => null,
            'last_failed_at' => null,
            'last_error' => null,
            'etag_header' => $result->etag,
            'last_modified_header' => $result->lastModified,
            'next_check_at' => Date::now()->addMinutes(self::REFRESH_INTERVAL_MINUTES),
            'consecutive_failures' => 0,
            'disabled_at' => null,
        ])->save();
    }

    private function onNotModified(Feed $feed, NotModified $result): void
    {
        $feed->forceFill([
            'last_fetched_at' => Date::now(),
            'last_dispatched_at' => null,
            'last_failed_at' => null,
            'last_error' => null,
            'next_check_at' => Date::now()->addMinutes(self::REFRESH_INTERVAL_MINUTES),
            'consecutive_failures' => 0,
            'disabled_at' => null,
        ])->save();
    }

    private function onGone(Feed $feed): void
    {
        $feed->forceFill([
            'last_dispatched_at' => null,
            'last_error' => 'HTTP 410 Gone',
            'disabled_at' => Date::now(),
        ])->save();
    }

    private function onRateLimited(Feed $feed, RateLimited $result): void
    {
        $now = Date::now();
        $backoffStep = $this->backoffStepMinutes(max(1, (int) $feed->consecutive_failures));
        $retryAt = $now->copy()->addSeconds($result->retryAfterSeconds);
        $backoffAt = $now->copy()->addMinutes($backoffStep);
        $nextCheck = $retryAt->greaterThan($backoffAt) ? $retryAt : $backoffAt;

        $feed->forceFill([
            'last_dispatched_at' => null,
            'last_error' => "HTTP 429 (retry after {$result->retryAfterSeconds}s)",
            'next_check_at' => $nextCheck,
        ])->save();
    }

    private function onFailed(Feed $feed, Throwable $cause): void
    {
        $newCount = ((int) $feed->consecutive_failures) + 1;

        $attrs = [
            'last_dispatched_at' => null,
            'last_failed_at' => Date::now(),
            'last_error' => Str::limit($cause->getMessage(), 500, ''),
            'consecutive_failures' => $newCount,
            'next_check_at' => Date::now()->addMinutes($this->backoffStepMinutes($newCount)),
        ];

        if ($newCount >= self::MUTE_THRESHOLD) {
            $attrs['disabled_at'] = Date::now();
        }

        $feed->forceFill($attrs)->save();
    }

    private function backoffStepMinutes(int $failures): int
    {
        return match (true) {
            $failures <= 1 => 5,
            $failures === 2 => 15,
            $failures === 3 => 60,
            $failures === 4 => 360,
            default => 1440,
        };
    }
}

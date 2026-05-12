<?php

namespace App\Jobs;

use App\Data\Reader\ExtractionResult;
use App\Models\FeedEntry;
use App\Services\Feeds\BackoffSchedule;
use App\Services\Reader\ArticleExtractor;
use Illuminate\Contracts\Queue\ShouldBeUnique;
use Illuminate\Contracts\Queue\ShouldQueue;
use Illuminate\Foundation\Queue\Queueable;
use Illuminate\Support\Facades\Date;
use Throwable;

class ExtractArticleJob implements ShouldBeUnique, ShouldQueue
{
    use Queueable;

    public int $tries = 1;

    public int $uniqueFor = 600;

    public function __construct(public readonly int $feedEntryId) {}

    public function uniqueId(): string
    {
        return (string) $this->feedEntryId;
    }

    public function handle(ArticleExtractor $extractor): void
    {
        $entry = FeedEntry::query()->find($this->feedEntryId);
        if ($entry === null || ! is_string($entry->link) || $entry->link === '') {
            return;
        }

        // Stop re-attempting once we've hit the permanent-fail threshold.
        // The controller's gating already filters this case for the auto path,
        // but the job itself is the last line of defence.
        if (BackoffSchedule::isPermanentlyFailed((int) $entry->extraction_attempts)) {
            return;
        }

        $result = $extractor->extract($entry->link);

        $entry->forceFill(self::persistAttributes($entry, $result))->save();
    }

    public function failed(Throwable $e): void
    {
        try {
            $entry = FeedEntry::query()->find($this->feedEntryId);
        } catch (Throwable) {
            return;
        }

        if ($entry === null) {
            return;
        }

        $entry->forceFill([
            'extraction_attempts' => ((int) $entry->extraction_attempts) + 1,
            'extraction_attempted_at' => Date::now(),
            'extraction_failure_reason' => 'unreachable',
        ])->save();
    }

    /**
     * @return array<string, mixed>
     */
    private static function persistAttributes(FeedEntry $entry, ExtractionResult $result): array
    {
        $now = Date::now();
        $attempts = ((int) $entry->extraction_attempts) + 1;

        if ($result->isSuccess()) {
            return [
                'extracted_html' => $result->html,
                'extracted_at' => $now,
                'extraction_attempts' => $attempts,
                'extraction_attempted_at' => $now,
                'extraction_failure_reason' => null,
            ];
        }

        return [
            'extraction_attempts' => $attempts,
            'extraction_attempted_at' => $now,
            'extraction_failure_reason' => $result->failureReason?->value,
        ];
    }
}

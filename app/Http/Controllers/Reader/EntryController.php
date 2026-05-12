<?php

namespace App\Http\Controllers\Reader;

use App\Data\Reader\EntryReaderData;
use App\Data\Reader\ExtractionResult;
use App\Data\Reader\FeedReferenceData;
use App\Enums\ContentType;
use App\Enums\Reader\AutoChoice;
use App\Enums\Reader\ExtractionState;
use App\Http\Controllers\Controller;
use App\Jobs\ExtractArticleJob;
use App\Models\FeedEntry;
use App\Models\Subscription;
use App\Services\Feeds\BackoffSchedule;
use App\Services\Reader\ArticleExtractor;
use App\Services\Reader\AutoExtractDecider;
use Illuminate\Support\Facades\Date;
use Inertia\Inertia;
use Inertia\Response;

class EntryController extends Controller
{
    /**
     * Explicit column list — extracted_html only loads here, never via
     * EntriesQuery on the digest. Keep this scoped tightly so the column
     * cannot fan out via SELECT *.
     *
     * @var list<string>
     */
    private const ENTRY_COLUMNS = [
        'id',
        'feed_id',
        'entry_slug',
        'title',
        'link',
        'summary',
        'content',
        'author',
        'published_at',
        'content_type',
        'extracted_html',
        'extracted_at',
        'extraction_attempts',
        'extraction_attempted_at',
        'extraction_failure_reason',
    ];

    public function show(string $slug): Response
    {
        $entry = $this->resolveEntry($slug);

        $dispatched = false;
        if ($this->shouldDispatch($entry)) {
            ExtractArticleJob::dispatch($entry->id);
            $dispatched = true;
        }

        return Inertia::render('entry', [
            'entry' => $this->buildReader($entry, $dispatched),
        ]);
    }

    public function extract(string $slug, ArticleExtractor $extractor): Response
    {
        $entry = $this->resolveEntry($slug);

        // Manual path: bypass shouldAutoExtract and the backoff window. A
        // deliberate user click is always honoured. Only the "no link" guard
        // stays — without an upstream URL there's nothing to fetch.
        if (is_string($entry->link) && $entry->link !== '') {
            $result = $extractor->extract($entry->link);
            $entry->forceFill(self::persistAttributes($entry, $result))->save();
        }

        return Inertia::render('entry', [
            'entry' => $this->buildReader($entry, false),
        ]);
    }

    private function resolveEntry(string $slug): FeedEntry
    {
        $entry = FeedEntry::query()
            ->select(self::ENTRY_COLUMNS)
            ->with(['feed' => fn ($q) => $q->select(['id', 'feed_url', 'title', 'favicon_url'])])
            ->where('entry_slug', $slug)
            ->first();

        abort_if($entry === null, 404);
        abort_unless($entry->content_type === ContentType::Blogpost, 404);

        return $entry;
    }

    private function buildReader(FeedEntry $entry, bool $dispatched): EntryReaderData
    {
        $extractionState = self::deriveExtractionState($entry, $dispatched);
        $autoChoice = $extractionState === ExtractionState::Available
            ? AutoChoice::Extracted
            : AutoChoice::Feed;

        $feed = $entry->feed;

        return new EntryReaderData(
            entrySlug: (string) $entry->entry_slug,
            title: $entry->title,
            author: $entry->author,
            publishedAt: $entry->published_at,
            sourceUrl: $entry->link,
            sourceDomain: self::deriveDomain($entry->link),
            feed: new FeedReferenceData(
                displayTitle: Subscription::resolveDisplayTitle(
                    customTitle: null,
                    pdsTitle: null,
                    feedTitle: $feed?->title,
                    feedUrl: $feed?->feed_url,
                ),
                faviconUrl: $feed?->favicon_url ?? self::deriveFaviconUrl((string) ($feed?->feed_url ?? '')),
            ),
            feedBody: $entry->content,
            extractedBody: $entry->extracted_html,
            autoChoice: $autoChoice,
            extractionState: $extractionState,
        );
    }

    private function shouldDispatch(FeedEntry $entry): bool
    {
        if (! AutoExtractDecider::shouldAutoExtract($entry)) {
            return false;
        }

        if ($entry->extracted_html !== null && $entry->extracted_html !== '') {
            return false;
        }

        if (BackoffSchedule::isPermanentlyFailed((int) $entry->extraction_attempts)) {
            return false;
        }

        if ($entry->link === null || $entry->link === '') {
            return false;
        }

        return self::backoffWindowElapsed($entry);
    }

    private static function backoffWindowElapsed(FeedEntry $entry): bool
    {
        $attempted = $entry->extraction_attempted_at;
        $attempts = (int) $entry->extraction_attempts;

        if ($attempted === null || $attempts === 0) {
            return true;
        }

        $waitMinutes = BackoffSchedule::stepMinutes($attempts);

        return $attempted->copy()->addMinutes($waitMinutes)->lessThanOrEqualTo(Date::now());
    }

    private static function deriveExtractionState(FeedEntry $entry, bool $justDispatched): ExtractionState
    {
        if ($entry->extracted_html !== null && $entry->extracted_html !== '') {
            return ExtractionState::Available;
        }

        if ($justDispatched) {
            return ExtractionState::Pending;
        }

        if ($entry->extraction_failure_reason !== null) {
            return ExtractionState::Failed;
        }

        return ExtractionState::NotAttempted;
    }

    /**
     * Mirrors ExtractArticleJob::persistAttributes — the manual path writes
     * the same shape as the queued one. Kept duplicated rather than extracted
     * to keep both call sites readable; if a third caller appears, promote.
     *
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

    private static function deriveDomain(?string $url): ?string
    {
        if ($url === null || $url === '') {
            return null;
        }

        $host = parse_url($url, PHP_URL_HOST);
        if (! is_string($host) || $host === '') {
            return null;
        }

        return str_starts_with($host, 'www.') ? substr($host, 4) : $host;
    }

    /**
     * Mirrors FeedEntryViewData::deriveFaviconUrl so the reader gets the same
     * fallback shape consume rows already use.
     */
    private static function deriveFaviconUrl(string $feedUrl): ?string
    {
        $parts = parse_url($feedUrl);
        if ($parts === false || ! isset($parts['scheme'], $parts['host'])) {
            return null;
        }

        return $parts['scheme'].'://'.$parts['host'].'/favicon.ico';
    }
}

<?php

namespace App\Data\Feeds;

use App\Enums\ContentType;
use App\Models\Subscription;
use Carbon\CarbonImmutable;
use Spatie\LaravelData\Attributes\MapOutputName;
use Spatie\LaravelData\Data;
use Spatie\LaravelData\Mappers\SnakeCaseMapper;
use Spatie\TypeScriptTransformer\Attributes\TypeScript;

#[TypeScript]
#[MapOutputName(SnakeCaseMapper::class)]
class FeedEntryViewData extends Data
{
    public function __construct(
        public int $id,
        public int $feedId,
        public string $entrySlug,
        public ?string $displayTitle,
        public ?string $entryTitle,
        public ?string $link,
        public ?string $summary,
        public ?string $author,
        public ?CarbonImmutable $publishedAt,
        public CarbonImmutable $firstSeenAt,
        public ContentType $contentType,
        public ?string $faviconUrl,
    ) {}

    public static function fromRow(object $row): self
    {
        $contentType = $row->content_type instanceof ContentType
            ? $row->content_type
            : ContentType::from((string) $row->content_type);

        // Legacy microblog rows ingested before MicroblogBodyNormalizer existed
        // have content populated but summary null; the renderer only reads
        // summary, so backfill at read time without a migration.
        $summary = $row->summary;
        if (
            $contentType === ContentType::Microblog
            && ($summary === null || $summary === '')
            && ! empty($row->content ?? null)
        ) {
            $summary = $row->content;
        }

        return new self(
            id: (int) $row->id,
            feedId: (int) $row->feed_id,
            entrySlug: (string) $row->entry_slug,
            displayTitle: Subscription::resolveDisplayTitle(
                $row->custom_title,
                $row->pds_title,
                $row->feed_title,
                $row->feed_url,
            ),
            entryTitle: $row->entry_title,
            link: $row->link,
            summary: $summary,
            author: $row->author,
            publishedAt: $row->published_at !== null ? CarbonImmutable::parse($row->published_at) : null,
            firstSeenAt: CarbonImmutable::parse($row->first_seen_at),
            contentType: $contentType,
            faviconUrl: $row->favicon_url ?? self::deriveFaviconUrl((string) $row->feed_url),
        );
    }

    /**
     * Fallback when favicon_url isn't populated yet (new feed before first
     * refresh, or discovery legitimately failed).
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

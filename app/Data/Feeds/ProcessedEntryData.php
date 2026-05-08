<?php

namespace App\Data\Feeds;

use App\Enums\ContentType;
use Carbon\CarbonImmutable;

final readonly class ProcessedEntryData
{
    /**
     * @param  list<FeedEnclosureData>  $enclosures
     * @param  array<string, mixed>  $metadata
     */
    public function __construct(
        public ?string $title,
        public ?string $link,
        public ?string $guid,
        public ?string $summary,
        public ?string $content,
        public ?string $author,
        public ?CarbonImmutable $publishedAt,
        public ContentType $contentType,
        public array $enclosures,
        public array $metadata,
    ) {}

    public static function fromFetched(FetchedEntryData $entry): self
    {
        return new self(
            title: $entry->title,
            link: $entry->link,
            guid: $entry->guid,
            summary: $entry->summary,
            content: $entry->content,
            author: $entry->author,
            publishedAt: $entry->publishedAt,
            contentType: ContentType::Blogpost,
            enclosures: $entry->enclosures ?? [],
            metadata: [],
        );
    }

    public function withContentType(ContentType $contentType): self
    {
        return new self(
            title: $this->title,
            link: $this->link,
            guid: $this->guid,
            summary: $this->summary,
            content: $this->content,
            author: $this->author,
            publishedAt: $this->publishedAt,
            contentType: $contentType,
            enclosures: $this->enclosures,
            metadata: $this->metadata,
        );
    }

    public function withSanitizedContent(?string $content, ?string $summary): self
    {
        return new self(
            title: $this->title,
            link: $this->link,
            guid: $this->guid,
            summary: $summary,
            content: $content,
            author: $this->author,
            publishedAt: $this->publishedAt,
            contentType: $this->contentType,
            enclosures: $this->enclosures,
            metadata: $this->metadata,
        );
    }
}

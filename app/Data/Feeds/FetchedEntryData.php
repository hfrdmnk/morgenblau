<?php

namespace App\Data\Feeds;

use Carbon\CarbonImmutable;
use Spatie\LaravelData\Attributes\DataCollectionOf;
use Spatie\LaravelData\Attributes\MapOutputName;
use Spatie\LaravelData\Data;
use Spatie\LaravelData\Mappers\SnakeCaseMapper;
use Spatie\TypeScriptTransformer\Attributes\TypeScript;

#[TypeScript]
#[MapOutputName(SnakeCaseMapper::class)]
class FetchedEntryData extends Data
{
    /**
     * @param  list<FeedEnclosureData>|null  $enclosures
     */
    public function __construct(
        public ?string $title,
        public ?string $link,
        public ?string $guid,
        public ?string $summary,
        public ?string $content,
        public ?string $author,
        public ?CarbonImmutable $publishedAt,
        #[DataCollectionOf(FeedEnclosureData::class)]
        public ?array $enclosures = null,
    ) {}
}

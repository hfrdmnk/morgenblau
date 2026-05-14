<?php

namespace App\Data\Reader;

use Carbon\CarbonImmutable;
use Spatie\LaravelData\Attributes\MapOutputName;
use Spatie\LaravelData\Data;
use Spatie\LaravelData\Mappers\SnakeCaseMapper;
use Spatie\TypeScriptTransformer\Attributes\TypeScript;

#[TypeScript]
#[MapOutputName(SnakeCaseMapper::class)]
class WatchReaderData extends Data
{
    public function __construct(
        public string $entrySlug,
        public ?string $title,
        public ?string $author,
        public ?CarbonImmutable $publishedAt,
        public ?string $sourceUrl,
        public ?string $sourceDomain,
        public FeedReferenceData $feed,
        public ?string $description,
        public ?string $embedUrl,
        public ?string $thumbnailUrl,
    ) {}
}

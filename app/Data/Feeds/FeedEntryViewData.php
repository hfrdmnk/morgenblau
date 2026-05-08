<?php

namespace App\Data\Feeds;

use App\Enums\ContentType;
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
        public ?string $displayTitle,
        public ?string $entryTitle,
        public ?string $link,
        public ?string $summary,
        public ?string $author,
        public ?CarbonImmutable $publishedAt,
        public CarbonImmutable $firstSeenAt,
        public ContentType $contentType,
    ) {}
}

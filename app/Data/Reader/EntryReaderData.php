<?php

namespace App\Data\Reader;

use App\Enums\Reader\AutoChoice;
use App\Enums\Reader\ExtractionState;
use Carbon\CarbonImmutable;
use Spatie\LaravelData\Attributes\MapOutputName;
use Spatie\LaravelData\Data;
use Spatie\LaravelData\Mappers\SnakeCaseMapper;
use Spatie\TypeScriptTransformer\Attributes\TypeScript;

#[TypeScript]
#[MapOutputName(SnakeCaseMapper::class)]
class EntryReaderData extends Data
{
    public function __construct(
        public string $entrySlug,
        public ?string $title,
        public ?string $author,
        public ?CarbonImmutable $publishedAt,
        public ?string $sourceUrl,
        public ?string $sourceDomain,
        public FeedReferenceData $feed,
        public ?string $feedBody,
        public ?string $extractedBody,
        public AutoChoice $autoChoice,
        public ExtractionState $extractionState,
    ) {}
}

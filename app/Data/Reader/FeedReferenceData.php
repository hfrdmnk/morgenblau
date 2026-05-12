<?php

namespace App\Data\Reader;

use Spatie\LaravelData\Attributes\MapOutputName;
use Spatie\LaravelData\Data;
use Spatie\LaravelData\Mappers\SnakeCaseMapper;
use Spatie\TypeScriptTransformer\Attributes\TypeScript;

#[TypeScript]
#[MapOutputName(SnakeCaseMapper::class)]
class FeedReferenceData extends Data
{
    public function __construct(
        public string $displayTitle,
        public ?string $faviconUrl,
    ) {}
}

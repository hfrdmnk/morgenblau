<?php

namespace App\Data\Feeds;

use App\Enums\SourceType;
use Spatie\LaravelData\Attributes\MapInputName;
use Spatie\LaravelData\Data;
use Spatie\LaravelData\Mappers\SnakeCaseMapper;
use Spatie\TypeScriptTransformer\Attributes\TypeScript;

#[TypeScript]
#[MapInputName(SnakeCaseMapper::class)]
class ChosenFeedData extends Data
{
    public function __construct(
        public string $feedUrl,
        public ?string $title,
        public ?string $siteUrl,
        public SourceType $sourceType,
    ) {}
}

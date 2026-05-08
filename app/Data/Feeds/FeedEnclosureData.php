<?php

namespace App\Data\Feeds;

use Spatie\LaravelData\Attributes\MapOutputName;
use Spatie\LaravelData\Data;
use Spatie\LaravelData\Mappers\SnakeCaseMapper;
use Spatie\TypeScriptTransformer\Attributes\TypeScript;

#[TypeScript]
#[MapOutputName(SnakeCaseMapper::class)]
class FeedEnclosureData extends Data
{
    public function __construct(
        public string $url,
        public ?string $type,
        public ?int $length,
    ) {}
}

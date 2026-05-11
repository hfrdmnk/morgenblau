<?php

namespace App\Data\Digest;

use Spatie\LaravelData\Attributes\MapOutputName;
use Spatie\LaravelData\Data;
use Spatie\LaravelData\Mappers\SnakeCaseMapper;
use Spatie\TypeScriptTransformer\Attributes\TypeScript;

#[TypeScript]
#[MapOutputName(SnakeCaseMapper::class)]
class DigestStatusData extends Data
{
    public function __construct(
        public bool $pending,
        public int $newCount,
        public ?string $latestEntryAt,
    ) {}
}

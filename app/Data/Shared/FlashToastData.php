<?php

namespace App\Data\Shared;

use App\Enums\FlashToastType;
use Spatie\LaravelData\Attributes\MapOutputName;
use Spatie\LaravelData\Data;
use Spatie\LaravelData\Mappers\SnakeCaseMapper;
use Spatie\TypeScriptTransformer\Attributes\TypeScript;

#[TypeScript]
#[MapOutputName(SnakeCaseMapper::class)]
class FlashToastData extends Data
{
    public function __construct(
        public FlashToastType $type,
        public string $message,
    ) {}
}

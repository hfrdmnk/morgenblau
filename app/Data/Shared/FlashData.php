<?php

namespace App\Data\Shared;

use Spatie\LaravelData\Attributes\MapOutputName;
use Spatie\LaravelData\Data;
use Spatie\LaravelData\Mappers\SnakeCaseMapper;
use Spatie\LaravelData\Optional;
use Spatie\TypeScriptTransformer\Attributes\TypeScript;

#[TypeScript]
#[MapOutputName(SnakeCaseMapper::class)]
class FlashData extends Data
{
    public function __construct(
        public string|Optional $message,
        public FlashToastData|Optional $toast,
    ) {}
}

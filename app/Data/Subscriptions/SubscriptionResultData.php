<?php

namespace App\Data\Subscriptions;

use Spatie\LaravelData\Attributes\MapOutputName;
use Spatie\LaravelData\Data;
use Spatie\LaravelData\Mappers\SnakeCaseMapper;
use Spatie\TypeScriptTransformer\Attributes\TypeScript;

#[TypeScript]
#[MapOutputName(SnakeCaseMapper::class)]
class SubscriptionResultData extends Data
{
    public function __construct(
        public string $title,
        public string $atUri,
    ) {}
}

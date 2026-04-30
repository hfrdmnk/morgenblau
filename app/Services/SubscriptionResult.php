<?php

namespace App\Services;

final class SubscriptionResult
{
    public function __construct(
        public readonly string $title,
        public readonly ?string $atUri,
    ) {}
}

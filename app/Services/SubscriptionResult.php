<?php

namespace App\Services;

use App\Models\Subscription;

final class SubscriptionResult
{
    public function __construct(
        public readonly string $title,
        public readonly bool $isPrivate,
        public readonly ?string $atUri,
        public readonly ?Subscription $subscription,
    ) {}
}

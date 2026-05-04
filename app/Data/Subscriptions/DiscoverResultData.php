<?php

namespace App\Data\Subscriptions;

use App\Data\Feeds\ResolvedFeedData;
use Spatie\LaravelData\Attributes\DataCollectionOf;
use Spatie\LaravelData\Attributes\MapOutputName;
use Spatie\LaravelData\Data;
use Spatie\LaravelData\DataCollection;
use Spatie\LaravelData\Mappers\SnakeCaseMapper;
use Spatie\TypeScriptTransformer\Attributes\TypeScript;

#[TypeScript]
#[MapOutputName(SnakeCaseMapper::class)]
class DiscoverResultData extends Data
{
    public function __construct(
        /** @var DataCollection<int, ResolvedFeedData> */
        #[DataCollectionOf(ResolvedFeedData::class)]
        public DataCollection $candidates,
        /** @var DataCollection<int, ExistingSubscriptionData> */
        #[DataCollectionOf(ExistingSubscriptionData::class)]
        public DataCollection $existingSubscriptions,
    ) {}
}

<?php

namespace App\Http\Controllers\Subscriptions;

use App\Data\Feeds\ChosenFeedData;
use App\Data\Feeds\ResolvedFeedData;
use App\Data\Shared\FlashToastData;
use App\Data\Subscriptions\DiscoverResultData;
use App\Data\Subscriptions\SubscriptionResultData;
use App\Enums\FlashToastType;
use App\Exceptions\AlreadySubscribedException;
use App\Exceptions\PdsWriteException;
use App\Exceptions\UnresolvableFeedException;
use App\Http\Controllers\Controller;
use App\Http\Requests\Subscriptions\DiscoverRequest;
use App\Http\Requests\Subscriptions\StoreRequest;
use App\Services\Subscriptions\SubscriptionService;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\RedirectResponse;
use Illuminate\Validation\ValidationException;
use Inertia\Inertia;
use Spatie\LaravelData\DataCollection;

class SubscriptionController extends Controller
{
    public function __construct(private readonly SubscriptionService $subscriptions) {}

    public function discover(DiscoverRequest $request): JsonResponse
    {
        try {
            $candidates = $this->subscriptions->discover($request->validated()['url']);
        } catch (UnresolvableFeedException $e) {
            throw ValidationException::withMessages(['url' => $e->getMessage()]);
        }

        $result = new DiscoverResultData(
            candidates: new DataCollection(ResolvedFeedData::class, $candidates),
            existingSubscriptions: $this->subscriptions->listSubscriptions($request->user()),
        );

        return response()->json($result);
    }

    public function store(StoreRequest $request): RedirectResponse
    {
        $items = $request->validated()['subscriptions'];
        $user = $request->user();

        $choices = array_map(fn (array $item) => ChosenFeedData::from($item), $items);

        [$succeeded, $failed] = $this->subscriptions->createMany($user, $choices);

        // Surface duplicates as field errors so the React form keeps its
        // existing per-row validation contract.
        $duplicateErrors = [];
        foreach ($failed as $index => $row) {
            if ($row['error'] instanceof AlreadySubscribedException) {
                $duplicateErrors["subscriptions.{$index}.feed_url"] = 'You are already subscribed to this feed.';
            }
        }

        if ($duplicateErrors !== []) {
            throw ValidationException::withMessages($duplicateErrors);
        }

        // Other failures (PDS write errors, etc.) are surfaced in the flash
        // message and reported for ops follow-up.
        $writeFailures = array_values(array_filter(
            $failed,
            fn (array $row) => $row['error'] instanceof PdsWriteException,
        ));
        foreach ($writeFailures as $row) {
            report($row['error']);
        }

        $succeededTitles = array_map(fn (SubscriptionResultData $r) => $r->title, $succeeded);
        $failedTitles = array_map(
            fn (array $row) => $row['choice']->title ?: $row['choice']->feedUrl,
            $writeFailures,
        );

        Inertia::flash('toast', FlashToastData::from([
            'type' => $writeFailures === [] ? FlashToastType::Success : FlashToastType::Warning,
            'message' => $this->buildFlashMessage($succeededTitles, $failedTitles),
        ]));

        return back();
    }

    /**
     * @param  array<int, string>  $succeeded
     * @param  array<int, string>  $failed
     */
    private function buildFlashMessage(array $succeeded, array $failed): string
    {
        $successCount = count($succeeded);
        $parts = [];

        if ($successCount === 1) {
            $parts[] = "Subscribed to {$succeeded[0]}.";
        } elseif ($successCount > 1) {
            $parts[] = "Subscribed to {$successCount} sources.";
        }

        if ($failed !== []) {
            $parts[] = 'Failed: '.implode(', ', $failed).'.';
        }

        return implode(' ', $parts);
    }
}

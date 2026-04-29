<?php

namespace App\Services;

use App\Models\User;
use Illuminate\Support\Facades\Cache;
use Revolution\Bluesky\Facades\Bluesky;
use Throwable;

class PdsProfileService
{
    /**
     * @return array{avatar: ?string, displayName: ?string}|null
     */
    public function for(User $user): ?array
    {
        return Cache::remember(
            "pds-profile:{$user->did}",
            now()->addHour(),
            fn () => $this->fetch($user->did),
        );
    }

    /**
     * @return array{avatar: ?string, displayName: ?string}|null
     */
    private function fetch(string $did): ?array
    {
        try {
            $profile = Bluesky::public()->getProfile(actor: $did)->json();
        } catch (Throwable $e) {
            report($e);

            return null;
        }

        if (! is_array($profile)) {
            return null;
        }

        return [
            'avatar' => $profile['avatar'] ?? null,
            'displayName' => $profile['displayName'] ?? null,
        ];
    }
}

<?php

namespace App\Listeners;

use App\Models\User;
use Revolution\Bluesky\Events\OAuthSessionUpdated;

class PersistOAuthSession
{
    public function handle(OAuthSessionUpdated $event): void
    {
        $did = $event->session->did();

        if (empty($did)) {
            return;
        }

        User::where('did', $did)->update([
            'refresh_token' => $event->session->refresh(),
            'iss' => $event->session->issuer(),
        ]);

        session()->put('bluesky_session', $event->session->toArray());
    }
}

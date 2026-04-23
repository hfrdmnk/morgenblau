<?php

namespace App\Listeners;

use App\Models\User;
use Revolution\Bluesky\Events\OAuthSessionRefreshing;

class ClearRefreshTokenOnRotate
{
    public function handle(OAuthSessionRefreshing $event): void
    {
        $did = $event->session->did();

        if (empty($did)) {
            return;
        }

        User::where('did', $did)->update(['refresh_token' => '']);
    }
}

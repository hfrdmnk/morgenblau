<?php

namespace App\Listeners;

use App\Models\User;
use Illuminate\Support\Facades\Auth;
use Illuminate\Support\Facades\Log;
use Revolution\Bluesky\Events\OAuthSessionUpdated;

class PersistOAuthSession
{
    public function handle(OAuthSessionUpdated $event): void
    {
        $did = $event->session->did();

        if (empty($did)) {
            return;
        }

        $user = User::find($did);

        if ($user === null) {
            return;
        }

        $refresh = $event->session->refresh();
        $newIss = $event->session->issuer();

        // Reject silent iss changes for an existing user. A bug, account
        // migration, or attacker-influenced refresh shouldn't be able to
        // rewrite the auth server. Refresh still rotates so the session stays
        // usable; iss is preserved.
        $update = ['refresh_token' => $refresh];
        $issForMemory = $user->iss;
        if ($user->iss !== null && $newIss !== '' && $user->iss !== $newIss) {
            Log::warning('oauth iss change rejected', [
                'did' => $did,
                'old_iss' => $user->iss,
                'new_iss' => $newIss,
            ]);
        } else {
            $update['iss'] = $newIss;
            $issForMemory = $newIss;
        }

        // Route through the model so the 'refresh_token' encrypted cast applies.
        $user->update($update);

        session()->put('bluesky_session', $event->session->toArray());

        // Keep the currently-authenticated User instance in sync. Without this,
        // tokenForBluesky() merges the now-stale in-memory refresh_token back
        // over the rotated one, the response middleware writes that stale value
        // back here, and the consumed token poisons the next refresh.
        $current = Auth::user();
        if ($current instanceof User && $current->getKey() === $did) {
            $current->forceFill([
                'refresh_token' => $refresh,
                'iss' => $issForMemory,
            ])->syncOriginal();
        }
    }
}

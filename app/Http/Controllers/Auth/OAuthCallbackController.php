<?php

namespace App\Http\Controllers\Auth;

use App\Http\Controllers\Controller;
use App\Models\User;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Auth;
use Laravel\Socialite\Facades\Socialite;
use Revolution\Bluesky\Session\OAuthSession;

class OAuthCallbackController extends Controller
{
    public function __invoke(Request $request): RedirectResponse
    {
        $hint = $request->session()->pull('atproto.hint');

        /** @var \Laravel\Socialite\Two\User $oauthUser */
        $oauthUser = Socialite::driver('bluesky')
            ->setScopes(explode(' ', (string) config('bluesky.oauth.metadata.scope')))
            ->hint($hint)
            ->user();

        /** @var OAuthSession $session */
        $session = $oauthUser->session;

        $user = User::updateOrCreate(
            ['did' => $session->did()],
            [
                'refresh_token' => $session->refresh(),
                'iss' => $session->issuer(),
            ],
        );

        // ATProto access tokens are opaque (not JWTs) on most PDSes, so the
        // package's JWT-exp expiry check would treat them as expired on every
        // request. Compute expires_at from the spec-mandated expires_in and let
        // PersistableOAuthSession::tokenExpired() consult it instead.
        $session->put('expires_at', now()->getTimestamp() + (int) ($session->get('expires_in') ?: 1800));

        $request->session()->put('bluesky_session', $session->toArray());
        $request->session()->put('atproto.handle', $session->handle());

        Auth::login($user, remember: true);

        return redirect()->intended(route('consume'));
    }
}

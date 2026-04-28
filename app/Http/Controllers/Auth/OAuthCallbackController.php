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

        $request->session()->put('bluesky_session', $session->toArray());
        $request->session()->put('atproto.handle', $session->handle());

        Auth::login($user, remember: true);

        return to_route('dashboard');
    }
}

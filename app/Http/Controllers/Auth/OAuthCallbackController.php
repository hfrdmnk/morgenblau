<?php

namespace App\Http\Controllers\Auth;

use App\Http\Controllers\Controller;
use App\Models\User;
use App\Services\Feeds\FeedRefreshScheduler;
use App\Services\Subscriptions\SubscriptionService;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Auth;
use Illuminate\Support\Facades\Log;
use Laravel\Socialite\Facades\Socialite;
use Revolution\Bluesky\Session\OAuthSession;
use Throwable;

class OAuthCallbackController extends Controller
{
    public function __construct(
        private readonly SubscriptionService $subscriptions,
        private readonly FeedRefreshScheduler $scheduler,
    ) {}

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

        // Capture before dispatch so the deferred-entries wait
        // (last_dispatched_at >= since) holds for every feed this request stamps.
        $actionAt = now()->toIso8601String();

        try {
            $this->subscriptions->reconcile($user);
        } catch (Throwable $e) {
            Log::warning('subscriptions reconcile on login failed', [
                'did' => $user->did,
                'error' => $e->getMessage(),
            ]);
        }

        $this->scheduler->dispatchForUser($user);

        $request->session()->put('fetch_action_at', $actionAt);

        return redirect()->intended(route('consume'));
    }
}

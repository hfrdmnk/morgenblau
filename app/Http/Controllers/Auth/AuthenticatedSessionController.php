<?php

namespace App\Http\Controllers\Auth;

use App\Http\Controllers\Controller;
use Illuminate\Http\Client\ConnectionException;
use Illuminate\Http\Client\RequestException;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Auth;
use Illuminate\Validation\ValidationException;
use Inertia\Inertia;
use Inertia\Response;
use Laravel\Socialite\Facades\Socialite;
use Symfony\Component\HttpFoundation\Response as HttpResponse;

class AuthenticatedSessionController extends Controller
{
    public function create(): Response
    {
        return Inertia::render('auth/login');
    }

    public function store(Request $request): HttpResponse
    {
        $validated = $request->validate([
            'handle' => ['nullable', 'string', 'max:253'],
        ]);

        $hint = $validated['handle'] ?? null;
        $request->session()->put('atproto.hint', $hint);

        try {
            // Inertia submits the login form via XHR. Returning a Symfony redirect
            // would make the XHR follow the 302 cross-origin to the OAuth provider
            // and fail CORS. Inertia::location triggers a full browser navigation
            // instead via the X-Inertia-Location header.
            $redirect = Socialite::driver('bluesky')
                ->setScopes(explode(' ', (string) config('bluesky.oauth.metadata.scope')))
                ->hint($hint)
                ->redirect();
        } catch (ConnectionException) {
            throw ValidationException::withMessages([
                'handle' => "Couldn't reach that account's server. Check the handle and try again.",
            ]);
        } catch (RequestException) {
            throw ValidationException::withMessages([
                'handle' => "That handle doesn't look right. Try your full handle, e.g. alice.bsky.social.",
            ]);
        }

        return Inertia::location($redirect->getTargetUrl());
    }

    public function destroy(Request $request): RedirectResponse
    {
        Auth::logout();

        $request->session()->invalidate();
        $request->session()->regenerateToken();

        return redirect('/');
    }
}

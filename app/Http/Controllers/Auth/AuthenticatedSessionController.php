<?php

namespace App\Http\Controllers\Auth;

use App\Http\Controllers\Controller;
use Illuminate\Http\RedirectResponse;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Auth;
use Inertia\Inertia;
use Inertia\Response;
use Laravel\Socialite\Facades\Socialite;
use Symfony\Component\HttpFoundation\RedirectResponse as SymfonyRedirectResponse;

class AuthenticatedSessionController extends Controller
{
    public function create(): Response
    {
        return Inertia::render('auth/login');
    }

    public function store(Request $request): SymfonyRedirectResponse
    {
        $validated = $request->validate([
            'handle' => ['nullable', 'string', 'max:253'],
        ]);

        $hint = $validated['handle'] ?? null;
        $request->session()->put('atproto.hint', $hint);

        return Socialite::driver('bluesky')
            ->setScopes(explode(' ', (string) config('bluesky.oauth.metadata.scope')))
            ->hint($hint)
            ->redirect();
    }

    public function destroy(Request $request): RedirectResponse
    {
        Auth::logout();

        $request->session()->invalidate();
        $request->session()->regenerateToken();

        return redirect('/');
    }
}

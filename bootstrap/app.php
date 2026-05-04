<?php

use App\Http\Middleware\EnsureBlueskySession;
use App\Http\Middleware\HandleAppearance;
use App\Http\Middleware\HandleInertiaRequests;
use Illuminate\Auth\AuthenticationException;
use Illuminate\Foundation\Application;
use Illuminate\Foundation\Configuration\Exceptions;
use Illuminate\Foundation\Configuration\Middleware;
use Illuminate\Http\Middleware\AddLinkHeadersForPreloadedAssets;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Auth;
use Illuminate\Support\Facades\Log;
use Inertia\Inertia;

return Application::configure(basePath: dirname(__DIR__))
    ->withRouting(
        web: __DIR__.'/../routes/web.php',
        commands: __DIR__.'/../routes/console.php',
        health: '/up',
    )
    ->withMiddleware(function (Middleware $middleware): void {
        $middleware->encryptCookies(except: ['appearance', 'sidebar_state']);

        $middleware->web(append: [
            HandleAppearance::class,
            HandleInertiaRequests::class,
            AddLinkHeadersForPreloadedAssets::class,
            EnsureBlueskySession::class,
        ]);
    })
    ->withExceptions(function (Exceptions $exceptions): void {
        $exceptions->render(function (AuthenticationException $e, Request $request) {
            Log::warning('oauth session ended', [
                'path' => $request->path(),
                'method' => $request->method(),
                'authed' => Auth::check(),
                'has_bluesky_session' => $request->session()->has('bluesky_session'),
                'user_updated_at' => Auth::user()?->updated_at?->toIso8601String(),
                'reason' => $e->getPrevious()?->getMessage(),
            ]);

            // If we're inside a request that *was* authenticated (e.g. PDS refresh
            // failed mid-call), tear the session down so the next request really is
            // logged out and the stale remember cookie can't reauthenticate us back
            // into a broken state.
            if (Auth::check()) {
                Auth::logout();
                $request->session()->invalidate();
                $request->session()->regenerateToken();
            }

            // After invalidate(), so the stash survives. Only stash safe methods —
            // we don't want to redirect a user back to a destructive POST.
            if ($request->isMethodSafe()) {
                $request->session()->put('url.intended', $request->fullUrl());
            }

            $request->session()->flash('flash', [
                'message' => 'Your session expired — please sign in again.',
            ]);

            if ($request->header('X-Inertia')) {
                return Inertia::location(route('login'));
            }

            if ($request->expectsJson() || $request->ajax()) {
                return response()->json(['message' => 'Session expired'], 401);
            }

            return redirect()->route('login');
        });
    })->create();

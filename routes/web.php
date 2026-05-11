<?php

use App\Http\Controllers\Auth\AuthenticatedSessionController;
use App\Http\Controllers\Auth\OAuthCallbackController;
use App\Http\Controllers\ConsumeController;
use App\Http\Controllers\Feeds\FeedRefreshController;
use App\Http\Controllers\Subscriptions\SubscriptionController;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Auth;
use Illuminate\Support\Facades\Route;
use Inertia\Inertia;
use Revolution\Bluesky\Socialite\Http\OAuthMetaController;

Route::get('/', function (Request $request) {
    // In local dev the package hardcodes the OAuth redirect URI to
    // http://127.0.0.1:8000/ (BlueskyServiceProvider::socialite()). Intercept
    // the ?iss callback here and forward to the real handler.
    if (app()->isLocal() && $request->has('iss')) {
        return to_route('bluesky.oauth.redirect', $request->query());
    }

    if (Auth::check()) {
        return redirect()->route('consume');
    }

    return Inertia::render('welcome');
})->name('home');

Route::middleware('guest')->group(function () {
    Route::get('login', [AuthenticatedSessionController::class, 'create'])->name('login');
    Route::post('login', [AuthenticatedSessionController::class, 'store']);
});

Route::post('logout', [AuthenticatedSessionController::class, 'destroy'])
    ->middleware('auth')
    ->name('logout');

// Canonical feat-auth-aligned OAuth endpoints. The package's own
// /bluesky/oauth/{client-metadata,jwks}.json routes still exist (harmless).
Route::get('oauth-client-metadata.json', [OAuthMetaController::class, 'clientMetadata'])
    ->name('oauth.client-metadata');
Route::get('oauth-jwks.json', [OAuthMetaController::class, 'jwks'])
    ->name('oauth.jwks');
Route::get('oauth/callback', OAuthCallbackController::class)
    ->name('bluesky.oauth.redirect');

Route::middleware(['auth'])->group(function () {
    Route::inertia('discover', 'discover')->name('discover');
    Route::get('consume', ConsumeController::class)->name('consume');
    Route::inertia('create', 'create')->name('create');

    Route::post('subscriptions/discover', [SubscriptionController::class, 'discover'])
        ->middleware('throttle:subscriptions')
        ->name('subscriptions.discover');
    Route::post('subscriptions', [SubscriptionController::class, 'store'])
        ->middleware('throttle:subscriptions')
        ->name('subscriptions.store');

    Route::post('feeds/refresh', FeedRefreshController::class)
        ->middleware('throttle:6,1')
        ->name('feeds.refresh');
});

require __DIR__.'/settings.php';

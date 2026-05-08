<?php

namespace App\Providers;

use App\Services\Feeds\Adapters\PodcastAdapter;
use App\Services\Feeds\Adapters\WebsiteAdapter;
use App\Services\Feeds\Adapters\YouTubeAdapter;
use App\Services\Feeds\FeedResolver;
use App\Services\Feeds\OutboundFeedClient;
use App\Services\Http\DnsResolver;
use App\Services\Http\SystemDnsResolver;
use Carbon\CarbonImmutable;
use FeedIo\FeedIo;
use Illuminate\Cache\RateLimiting\Limit;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\Date;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\RateLimiter;
use Illuminate\Support\Facades\URL;
use Illuminate\Support\ServiceProvider;
use Illuminate\Support\Str;
use Revolution\Bluesky\Socialite\OAuthConfig;

class AppServiceProvider extends ServiceProvider
{
    public function register(): void
    {
        $this->app->bind(DnsResolver::class, SystemDnsResolver::class);

        $this->app->bind(ConditionalFeedClient::class, OutboundFeedClient::class);

        $this->app->bind(FeedIo::class, fn ($app) => new FeedIo($app->make(OutboundFeedClient::class)));

        $this->app->singleton(FeedResolver::class, fn ($app) => new FeedResolver([
            $app->make(YouTubeAdapter::class),
            $app->make(PodcastAdapter::class),
            $app->make(WebsiteAdapter::class),
        ]));
    }

    public function boot(): void
    {
        $this->configureDefaults();
        $this->configureBluesky();
        $this->configureRateLimiting();
    }

    protected function configureRateLimiting(): void
    {
        RateLimiter::for(
            'subscriptions',
            fn (Request $request) => Limit::perMinute(50)->by($request->user()?->getKey() ?: $request->ip()),
        );
    }

    protected function configureDefaults(): void
    {
        Date::use(CarbonImmutable::class);

        DB::prohibitDestructiveCommands(
            app()->isProduction(),
        );

        if (Str::startsWith((string) config('app.url'), 'https://')) {
            URL::forceScheme('https');
        }
    }

    /**
     * Wire a custom client-metadata document served at /oauth-client-metadata.json.
     * Token-rotation listening lives in app/Listeners/PersistOAuthSession.php
     * (auto-registered via Laravel's listener discovery).
     */
    protected function configureBluesky(): void
    {
        OAuthConfig::clientMetadataUsing(function (): array {
            return collect(config('bluesky.oauth.metadata'))
                ->merge([
                    'client_id' => url('/oauth-client-metadata.json'),
                    'jwks_uri' => url('/oauth-jwks.json'),
                    'redirect_uris' => [url('/oauth/callback')],
                ])
                ->reject(fn ($v): bool => is_null($v))
                ->toArray();
        });
    }
}

<?php

namespace App\Providers;

use App\Listeners\PersistOAuthSession;
use Carbon\CarbonImmutable;
use Illuminate\Support\Facades\Date;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Event;
use Illuminate\Support\Facades\URL;
use Illuminate\Support\ServiceProvider;
use Illuminate\Support\Str;
use Revolution\Bluesky\Events\OAuthSessionUpdated;
use Revolution\Bluesky\Socialite\OAuthConfig;

class AppServiceProvider extends ServiceProvider
{
    public function boot(): void
    {
        $this->configureDefaults();
        $this->configureBluesky();
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
     * Wire Bluesky OAuth: event listeners for token rotation and a custom
     * client-metadata document served at /oauth-client-metadata.json.
     */
    protected function configureBluesky(): void
    {
        Event::listen(OAuthSessionUpdated::class, PersistOAuthSession::class);

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

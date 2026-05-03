<?php

namespace App\Models;

use Database\Factories\UserFactory;
use Illuminate\Database\Eloquent\Attributes\Fillable;
use Illuminate\Database\Eloquent\Attributes\Hidden;
use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Foundation\Auth\User as Authenticatable;
use Illuminate\Notifications\Notifiable;
use Illuminate\Support\Facades\Cache;
use Revolution\Bluesky\Contracts\Factory as BlueskyFactory;
use Revolution\Bluesky\Session\OAuthSession;
use Revolution\Bluesky\Traits\WithBluesky;

#[Fillable(['did', 'refresh_token', 'iss'])]
#[Hidden(['refresh_token', 'remember_token'])]
class User extends Authenticatable
{
    /** @use HasFactory<UserFactory> */
    use HasFactory, Notifiable;

    use WithBluesky {
        bluesky as protected baseBluesky;
    }

    protected $primaryKey = 'did';

    public $incrementing = false;

    protected $keyType = 'string';

    /**
     * @return array<string, string>
     */
    protected function casts(): array
    {
        return [
            'refresh_token' => 'encrypted',
        ];
    }

    public function bluesky(): BlueskyFactory
    {
        if (! $this->tokenForBluesky()->tokenExpired()) {
            return $this->baseBluesky();
        }

        return Cache::lock("bluesky:auth:{$this->did}", 10)->block(5, function (): BlueskyFactory {
            // Another request may have just refreshed under the lock.
            if (! $this->tokenForBluesky()->tokenExpired()) {
                return $this->baseBluesky();
            }

            return $this->baseBluesky()->refreshSession();
        });
    }

    protected function tokenForBluesky(): OAuthSession
    {
        $base = array_filter([
            'did' => $this->did,
            'refresh_token' => $this->refresh_token,
            'iss' => $this->iss,
        ]);

        $cached = session('bluesky_session');
        if (is_array($cached) && data_get($cached, 'did') === $this->did) {
            // Session has access_token, dpop nonces, didDoc, etc.
            // DB wins on overlap so the post-rotation refresh_token is canonical.
            return OAuthSession::create(array_merge($cached, $base));
        }

        return OAuthSession::create($base);
    }
}

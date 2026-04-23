<?php

namespace App\Models;

use Database\Factories\UserFactory;
use Illuminate\Database\Eloquent\Attributes\Fillable;
use Illuminate\Database\Eloquent\Attributes\Hidden;
use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Foundation\Auth\User as Authenticatable;
use Illuminate\Notifications\Notifiable;
use Revolution\Bluesky\Session\OAuthSession;
use Revolution\Bluesky\Traits\WithBluesky;

#[Fillable(['did', 'refresh_token', 'iss'])]
#[Hidden(['refresh_token', 'remember_token'])]
class User extends Authenticatable
{
    /** @use HasFactory<UserFactory> */
    use HasFactory, Notifiable, WithBluesky;

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

    protected function tokenForBluesky(): OAuthSession
    {
        return OAuthSession::create(array_filter([
            'did' => $this->did,
            'refresh_token' => $this->refresh_token,
            'iss' => $this->iss,
        ]));
    }
}

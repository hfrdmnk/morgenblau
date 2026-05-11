<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsToMany;
use Illuminate\Database\Eloquent\Relations\HasMany;
use Illuminate\Support\Facades\Date;
use Illuminate\Support\Str;
use Throwable;

class Feed extends Model
{
    protected $fillable = [
        'feed_url',
        'site_url',
        'title',
        'favicon_url',
        'favicon_checked_at',
        'last_fetched_at',
        'last_failed_at',
        'last_error',
        'last_dispatched_at',
        'etag_header',
        'last_modified_header',
        'next_check_at',
        'consecutive_failures',
        'disabled_at',
    ];

    private const REFRESH_INTERVAL_MINUTES = 30;

    private const MUTE_THRESHOLD = 20;

    /**
     * @return array<string, string>
     */
    protected function casts(): array
    {
        return [
            'last_fetched_at' => 'immutable_datetime',
            'last_failed_at' => 'immutable_datetime',
            'last_dispatched_at' => 'immutable_datetime',
            'next_check_at' => 'immutable_datetime',
            'disabled_at' => 'immutable_datetime',
            'favicon_checked_at' => 'immutable_datetime',
        ];
    }

    /**
     * @return HasMany<FeedEntry>
     */
    public function feedEntries(): HasMany
    {
        return $this->hasMany(FeedEntry::class);
    }

    /**
     * @return HasMany<Subscription>
     */
    public function subscriptions(): HasMany
    {
        return $this->hasMany(Subscription::class);
    }

    /**
     * @return BelongsToMany<User>
     */
    public function subscribers(): BelongsToMany
    {
        return $this->belongsToMany(User::class, 'subscriptions', 'feed_id', 'user_id');
    }

    public function markFetched(?string $etag, ?string $lastModified, int $intervalMinutes = self::REFRESH_INTERVAL_MINUTES): void
    {
        $now = Date::now();

        $this->forceFill([
            'last_fetched_at' => $now,
            'last_dispatched_at' => null,
            'last_failed_at' => null,
            'last_error' => null,
            'etag_header' => $etag,
            'last_modified_header' => $lastModified,
            'next_check_at' => $now->copy()->addMinutes($intervalMinutes),
            'consecutive_failures' => 0,
            'disabled_at' => null,
        ])->save();
    }

    public function markNotModified(int $intervalMinutes = self::REFRESH_INTERVAL_MINUTES): void
    {
        $now = Date::now();

        $this->forceFill([
            'last_fetched_at' => $now,
            'last_dispatched_at' => null,
            'last_failed_at' => null,
            'last_error' => null,
            'next_check_at' => $now->copy()->addMinutes($intervalMinutes),
            'consecutive_failures' => 0,
            'disabled_at' => null,
        ])->save();
    }

    public function markFailed(Throwable $cause): void
    {
        $now = Date::now();
        $newCount = ((int) $this->consecutive_failures) + 1;

        $attrs = [
            'last_dispatched_at' => null,
            'last_failed_at' => $now,
            'last_error' => Str::limit($cause->getMessage(), 500, ''),
            'consecutive_failures' => $newCount,
            'next_check_at' => $now->copy()->addMinutes(self::backoffStepMinutes($newCount)),
        ];

        if ($newCount >= self::MUTE_THRESHOLD) {
            $attrs['disabled_at'] = $now;
        }

        $this->forceFill($attrs)->save();
    }

    public function markRateLimited(int $retryAfterSeconds): void
    {
        $now = Date::now();
        $backoffStep = self::backoffStepMinutes(max(1, (int) $this->consecutive_failures));
        $retryAt = $now->copy()->addSeconds($retryAfterSeconds);
        $backoffAt = $now->copy()->addMinutes($backoffStep);

        $this->forceFill([
            'last_dispatched_at' => null,
            'last_error' => "HTTP 429 (retry after {$retryAfterSeconds}s)",
            'next_check_at' => $retryAt->greaterThan($backoffAt) ? $retryAt : $backoffAt,
        ])->save();
    }

    public function markGone(): void
    {
        $this->forceFill([
            'last_dispatched_at' => null,
            'last_error' => 'HTTP 410 Gone',
            'disabled_at' => Date::now(),
        ])->save();
    }

    private static function backoffStepMinutes(int $failures): int
    {
        return match (true) {
            $failures <= 1 => 5,
            $failures === 2 => 15,
            $failures === 3 => 60,
            $failures === 4 => 360,
            default => 1440,
        };
    }
}

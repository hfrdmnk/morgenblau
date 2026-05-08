<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsToMany;
use Illuminate\Database\Eloquent\Relations\HasMany;

class Feed extends Model
{
    protected $guarded = [];

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
}

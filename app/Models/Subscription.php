<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Attributes\Fillable;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

#[Fillable([
    'user_did',
    'feed_url',
    'title',
    'site_url',
    'category',
    'source_type',
    'is_private',
    'at_uri',
])]
class Subscription extends Model
{
    protected function casts(): array
    {
        return [
            'is_private' => 'boolean',
        ];
    }

    public function user(): BelongsTo
    {
        return $this->belongsTo(User::class, 'user_did', 'did');
    }
}

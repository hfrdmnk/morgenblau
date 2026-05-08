<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Casts\Attribute;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

class Subscription extends Model
{
    protected $fillable = [
        'user_id',
        'feed_id',
        'at_uri',
        'custom_title',
        'pds_title',
    ];

    /**
     * @return BelongsTo<User, $this>
     */
    public function user(): BelongsTo
    {
        return $this->belongsTo(User::class, 'user_id', 'did');
    }

    /**
     * @return BelongsTo<Feed, $this>
     */
    public function feed(): BelongsTo
    {
        return $this->belongsTo(Feed::class);
    }

    /**
     * @return Attribute<string, never>
     */
    protected function displayTitle(): Attribute
    {
        return Attribute::get(fn (): string => self::resolveDisplayTitle(
            $this->custom_title,
            $this->pds_title,
            $this->feed?->title,
            $this->feed?->feed_url,
        ));
    }

    public static function resolveDisplayTitle(?string $customTitle, ?string $pdsTitle, ?string $feedTitle, ?string $feedUrl): string
    {
        if ($customTitle !== null && $customTitle !== '') {
            return $customTitle;
        }

        if ($pdsTitle !== null && $pdsTitle !== '') {
            return $pdsTitle;
        }

        if ($feedTitle !== null && $feedTitle !== '') {
            return $feedTitle;
        }

        return $feedUrl ?? '';
    }
}

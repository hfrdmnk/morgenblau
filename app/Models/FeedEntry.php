<?php

namespace App\Models;

use App\Enums\ContentType;
use App\Services\Feeds\EntrySlugger;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

class FeedEntry extends Model
{
    protected static function booted(): void
    {
        // Safety net: production inserts go through FeedEntryUpserter (raw
        // upsert, no model events), but anything using FeedEntry::create —
        // factories, ad-hoc scripts — gets the slug computed for free.
        static::creating(function (self $entry): void {
            if (($entry->entry_slug ?? null) === null && $entry->feed_id !== null && $entry->guid !== null) {
                $entry->entry_slug = EntrySlugger::for((int) $entry->feed_id, (string) $entry->guid);
            }
        });
    }

    protected $fillable = [
        'feed_id',
        'guid',
        'entry_slug',
        'title',
        'link',
        'summary',
        'content',
        'author',
        'published_at',
        'content_type',
        'metadata',
        'first_seen_at',
        'updated_at',
        'extracted_html',
        'extracted_at',
        'extraction_attempts',
        'extraction_attempted_at',
        'extraction_failure_reason',
    ];

    public $timestamps = false;

    /**
     * @return array<string, string>
     */
    protected function casts(): array
    {
        return [
            'published_at' => 'immutable_datetime',
            'first_seen_at' => 'immutable_datetime',
            'updated_at' => 'immutable_datetime',
            'extracted_at' => 'immutable_datetime',
            'extraction_attempted_at' => 'immutable_datetime',
            'content_type' => ContentType::class,
            'metadata' => 'array',
        ];
    }

    /**
     * @return BelongsTo<Feed, $this>
     */
    public function feed(): BelongsTo
    {
        return $this->belongsTo(Feed::class);
    }
}

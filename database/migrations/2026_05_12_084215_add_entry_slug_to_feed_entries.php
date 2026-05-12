<?php

use App\Services\Feeds\EntrySlugger;
use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        // Two-step: add the column nullable so existing rows survive, backfill
        // a deterministic slug per row, then tighten the column to NOT NULL +
        // unique. Backfill is keyed on (feed_id, guid) so re-running the
        // migration produces identical slugs.
        Schema::table('feed_entries', function (Blueprint $table) {
            $table->string('entry_slug', 16)->nullable()->after('guid');
        });

        DB::table('feed_entries')
            ->select(['id', 'feed_id', 'guid'])
            ->orderBy('id')
            ->chunkById(1000, function ($rows): void {
                foreach ($rows as $row) {
                    DB::table('feed_entries')
                        ->where('id', $row->id)
                        ->update(['entry_slug' => EntrySlugger::for((int) $row->feed_id, (string) $row->guid)]);
                }
            });

        Schema::table('feed_entries', function (Blueprint $table) {
            $table->string('entry_slug', 16)->nullable(false)->change();
            $table->unique('entry_slug');
        });
    }

    public function down(): void
    {
        Schema::table('feed_entries', function (Blueprint $table) {
            $table->dropUnique(['entry_slug']);
            $table->dropColumn('entry_slug');
        });
    }
};

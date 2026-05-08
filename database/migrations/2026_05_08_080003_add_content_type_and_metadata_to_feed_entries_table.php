<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        Schema::table('feed_entries', function (Blueprint $table) {
            $table->string('content_type')->default('blogpost')->after('author');
            $table->json('metadata')->nullable()->after('content_type');

            $table->index('content_type');
        });
    }

    public function down(): void
    {
        Schema::table('feed_entries', function (Blueprint $table) {
            $table->dropIndex(['content_type']);
            $table->dropColumn(['content_type', 'metadata']);
        });
    }
};

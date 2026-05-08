<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        Schema::table('feeds', function (Blueprint $table) {
            $table->text('etag_header')->nullable()->after('last_dispatched_at');
            $table->text('last_modified_header')->nullable()->after('etag_header');
        });
    }

    public function down(): void
    {
        Schema::table('feeds', function (Blueprint $table) {
            $table->dropColumn(['etag_header', 'last_modified_header']);
        });
    }
};

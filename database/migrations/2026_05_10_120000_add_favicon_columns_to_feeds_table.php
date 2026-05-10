<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        Schema::table('feeds', function (Blueprint $table) {
            $table->string('favicon_url')->nullable()->after('title');
            $table->timestamp('favicon_checked_at')->nullable()->after('favicon_url');
        });
    }

    public function down(): void
    {
        Schema::table('feeds', function (Blueprint $table) {
            $table->dropColumn(['favicon_url', 'favicon_checked_at']);
        });
    }
};

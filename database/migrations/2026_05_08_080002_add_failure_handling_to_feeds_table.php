<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        Schema::table('feeds', function (Blueprint $table) {
            $table->unsignedInteger('consecutive_failures')->default(0)->after('next_check_at');
            $table->timestamp('disabled_at')->nullable()->index()->after('consecutive_failures');
        });
    }

    public function down(): void
    {
        Schema::table('feeds', function (Blueprint $table) {
            $table->dropIndex(['disabled_at']);
            $table->dropColumn(['consecutive_failures', 'disabled_at']);
        });
    }
};

<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        Schema::create('feeds', function (Blueprint $table) {
            $table->id();
            $table->string('feed_url')->unique();
            $table->string('site_url')->nullable();
            $table->string('title')->nullable();
            $table->timestamp('last_fetched_at')->nullable();
            $table->timestamp('last_failed_at')->nullable();
            $table->string('last_error', 500)->nullable();
            $table->timestamp('last_dispatched_at')->nullable();
            $table->timestamps();
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('feeds');
    }
};

<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    /**
     * Run the migrations.
     */
    public function up(): void
    {
        Schema::create('subscriptions', function (Blueprint $table) {
            $table->id();
            $table->string('user_did');
            $table->foreign('user_did')
                ->references('did')
                ->on('users')
                ->cascadeOnDelete();
            $table->string('feed_url', 2048);
            $table->string('title', 512)->nullable();
            $table->string('site_url', 2048)->nullable();
            $table->string('category', 128)->nullable();
            $table->string('source_type', 64)->default('rss');
            $table->boolean('is_private')->default(true);
            $table->string('at_uri', 2048)->nullable();
            $table->timestamps();

            $table->unique(['user_did', 'feed_url']);
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('subscriptions');
    }
};

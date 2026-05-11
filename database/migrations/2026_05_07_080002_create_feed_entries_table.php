<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        Schema::create('feed_entries', function (Blueprint $table) {
            $table->id();
            $table->foreignId('feed_id')->constrained('feeds')->cascadeOnDelete();
            $table->string('guid');
            $table->string('title')->nullable();
            $table->string('link')->nullable();
            $table->text('summary')->nullable();
            $table->longText('content')->nullable();
            $table->string('author')->nullable();
            $table->string('content_type')->default('blogpost');
            $table->json('metadata')->nullable();
            $table->timestamp('published_at')->nullable();
            $table->timestamp('first_seen_at');
            $table->timestamp('updated_at');

            $table->unique(['feed_id', 'guid']);
            $table->index(['feed_id', 'published_at']);
            $table->index(['published_at']);
            $table->index('content_type');
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('feed_entries');
    }
};

<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        Schema::create('subscriptions', function (Blueprint $table) {
            $table->id();
            $table->string('user_id');
            $table->foreign('user_id')->references('did')->on('users')->cascadeOnDelete();
            $table->foreignId('feed_id')->constrained('feeds')->cascadeOnDelete();
            $table->string('at_uri')->nullable();
            $table->string('custom_title')->nullable();
            // Mirror of value.title from the PDS record; honours the middle
            // tier of the display-title precedence ladder when custom_title
            // is absent.
            $table->string('pds_title')->nullable();
            $table->timestamps();
            $table->unique(['user_id', 'feed_id']);
        });
    }

    public function down(): void
    {
        Schema::dropIfExists('subscriptions');
    }
};

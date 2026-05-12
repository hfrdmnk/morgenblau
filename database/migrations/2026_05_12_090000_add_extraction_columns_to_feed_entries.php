<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up(): void
    {
        // Lazy readability cache: extracted_html is only ever SELECT'd by the
        // reader controller, never by EntriesQuery — guard against accidental
        // fan-out via SELECT *.
        Schema::table('feed_entries', function (Blueprint $table) {
            $table->longText('extracted_html')->nullable();
            $table->timestamp('extracted_at')->nullable();
            $table->unsignedInteger('extraction_attempts')->default(0);
            $table->timestamp('extraction_attempted_at')->nullable();
            $table->string('extraction_failure_reason')->nullable();
        });
    }

    public function down(): void
    {
        Schema::table('feed_entries', function (Blueprint $table) {
            $table->dropColumn([
                'extracted_html',
                'extracted_at',
                'extraction_attempts',
                'extraction_attempted_at',
                'extraction_failure_reason',
            ]);
        });
    }
};

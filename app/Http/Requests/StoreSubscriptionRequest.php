<?php

namespace App\Http\Requests;

use Illuminate\Contracts\Validation\ValidationRule;
use Illuminate\Foundation\Http\FormRequest;
use Illuminate\Validation\Rule;

class StoreSubscriptionRequest extends FormRequest
{
    public const SOURCE_TYPES = ['rss', 'video', 'podcast', 'microblog'];

    public function authorize(): bool
    {
        return $this->user() !== null;
    }

    /**
     * @return array<string, ValidationRule|array<mixed>|string>
     */
    public function rules(): array
    {
        return [
            'feed_url' => ['required', 'string', 'url:http,https', 'max:2048'],
            'title' => ['nullable', 'string', 'max:512'],
            'site_url' => ['nullable', 'string', 'url:http,https', 'max:2048'],
            'source_type' => ['required', Rule::in(self::SOURCE_TYPES)],
        ];
    }
}

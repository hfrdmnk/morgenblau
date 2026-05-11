<?php

namespace App\Http\Requests\Subscriptions;

use Illuminate\Contracts\Validation\ValidationRule;
use Illuminate\Foundation\Http\FormRequest;

class StoreRequest extends FormRequest
{
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
            'subscriptions' => ['required', 'array', 'min:1'],
            'subscriptions.*.feed_url' => ['required', 'string', 'url:http,https', 'max:2048', 'distinct:strict'],
            'subscriptions.*.title' => ['nullable', 'string', 'max:512'],
            'subscriptions.*.site_url' => ['nullable', 'string', 'url:http,https', 'max:2048'],
        ];
    }
}

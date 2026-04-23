<?php

namespace Database\Factories;

use App\Models\User;
use Illuminate\Database\Eloquent\Factories\Factory;

/**
 * @extends Factory<User>
 */
class UserFactory extends Factory
{
    /**
     * @return array<string, mixed>
     */
    public function definition(): array
    {
        return [
            'did' => 'did:plc:'.fake()->regexify('[a-z0-9]{24}'),
            'refresh_token' => fake()->sha256(),
            'iss' => 'https://bsky.social',
        ];
    }
}

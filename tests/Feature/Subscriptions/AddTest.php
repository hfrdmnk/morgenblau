<?php

use App\Models\User;
use Illuminate\Auth\AuthenticationException;
use Illuminate\Support\Facades\Auth;
use Illuminate\Support\Facades\Bus;
use Illuminate\Support\Facades\Http;

beforeEach(function () {
    Http::preventStrayRequests();
    Bus::fake();
});

test('guests cannot discover or store subscriptions', function () {
    $this->postJson(route('subscriptions.discover'), ['url' => 'https://example.com'])
        ->assertUnauthorized();

    $this->post(route('subscriptions.store'), [
        'subscriptions' => [[
            'feed_url' => 'https://example.com/rss.xml',
            'source_type' => 'rss',
        ]],
    ])->assertRedirect(route('login'));
});

test('discover returns every advertised feed in document order', function () {
    Http::fake([
        'example.com' => Http::response(
            '<html><head>'
            .'<title>Example</title>'
            .'<meta property="og:site_name" content="Example Blog">'
            .'<link rel="alternate" type="application/rss+xml" title="Main feed" href="/rss.xml">'
            .'<link rel="alternate" type="application/atom+xml" title="Comments" href="/comments.atom">'
            .'</head></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    $this->fakeListRecords($this->fakeBlueskyClient(), []);

    $this->actingAs(User::factory()->create());

    $this->postJson(route('subscriptions.discover'), [
        'url' => 'https://example.com',
    ])->assertOk()->assertExactJson([
        'candidates' => [
            [
                'feed_url' => 'https://example.com/rss.xml',
                'title' => 'Main feed',
                'site_url' => 'https://example.com',
                'source_type' => 'rss',
            ],
            [
                'feed_url' => 'https://example.com/comments.atom',
                'title' => 'Comments',
                'site_url' => 'https://example.com',
                'source_type' => 'rss',
            ],
        ],
        'existing_subscriptions' => [],
    ]);
});

test('discover preserves UTF-8 in feed and page titles', function () {
    Http::fake([
        'example.com' => Http::response(
            '<html><head>'
            .'<title>Café — Morgenblau</title>'
            .'<link rel="alternate" type="application/rss+xml" title="Café feed" href="/rss.xml">'
            .'</head></html>',
            200,
            ['Content-Type' => 'text/html; charset=utf-8'],
        ),
    ]);

    $this->fakeListRecords($this->fakeBlueskyClient(), []);

    $this->actingAs(User::factory()->create());

    $this->postJson(route('subscriptions.discover'), ['url' => 'https://example.com'])
        ->assertOk()
        ->assertJsonPath('candidates.0.title', 'Café feed');
});

test('discover falls back to og:site_name when a link has no title', function () {
    Http::fake([
        'example.com' => Http::response(
            '<html><head>'
            .'<meta property="og:site_name" content="Example Blog">'
            .'<link rel="alternate" type="application/rss+xml" href="/rss.xml">'
            .'</head></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    $this->fakeListRecords($this->fakeBlueskyClient(), []);

    $this->actingAs(User::factory()->create());

    $this->postJson(route('subscriptions.discover'), ['url' => 'https://example.com'])
        ->assertOk()
        ->assertJsonPath('candidates.0.title', 'Example Blog')
        ->assertJsonPath('candidates.0.feed_url', 'https://example.com/rss.xml');
});

test('discover surfaces feeds the user is already subscribed to with their saved titles', function () {
    Http::fake([
        'example.com' => Http::response(
            '<html><head>'
            .'<link rel="alternate" type="application/rss+xml" title="Main feed" href="/rss.xml">'
            .'<link rel="alternate" type="application/atom+xml" title="Comments" href="/comments.atom">'
            .'</head></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    $this->fakeListRecords($this->fakeBlueskyClient(), [
        ['feed_url' => 'https://example.com/rss.xml', 'title' => 'My nickname for this feed'],
    ]);

    $this->actingAs(User::factory()->create());

    $this->postJson(route('subscriptions.discover'), ['url' => 'https://example.com'])
        ->assertOk()
        ->assertJsonPath('existing_subscriptions', [
            [
                'feed_url' => 'https://example.com/rss.xml',
                'title' => 'My nickname for this feed',
                'custom_title' => null,
                'at_uri' => null,
            ],
        ]);
});

test('discover returns null title for existing subscriptions saved without one', function () {
    Http::fake([
        'example.com' => Http::response(
            '<html><head>'
            .'<link rel="alternate" type="application/rss+xml" title="Main feed" href="/rss.xml">'
            .'</head></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    $this->fakeListRecords($this->fakeBlueskyClient(), ['https://example.com/rss.xml']);

    $this->actingAs(User::factory()->create());

    $this->postJson(route('subscriptions.discover'), ['url' => 'https://example.com'])
        ->assertOk()
        ->assertJsonPath('existing_subscriptions', [
            [
                'feed_url' => 'https://example.com/rss.xml',
                'title' => null,
                'custom_title' => null,
                'at_uri' => null,
            ],
        ]);
});

test('discover returns a 422 with a url error when the page exposes no feed', function () {
    Http::fake([
        'example.com' => Http::response(
            '<html><head><title>Plain page</title></head><body>Nothing here.</body></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    $this->actingAs(freshenBluesky(User::factory()->create()));

    $this->postJson(route('subscriptions.discover'), ['url' => 'https://example.com'])
        ->assertStatus(422)
        ->assertJsonValidationErrors('url');
});

test('discover rejects a malformed URL', function () {
    $this->actingAs(freshenBluesky(User::factory()->create()));

    $this->postJson(route('subscriptions.discover'), ['url' => 'not-a-url'])
        ->assertStatus(422)
        ->assertJsonValidationErrors('url');
});

test('storing a subscription writes the chosen feed to the user PDS', function () {
    $client = $this->fakeBlueskyClient();
    $this->fakeListRecords($client, []);
    $client->shouldReceive('createRecord')
        ->once()
        ->withArgs(function (string $repo, string $collection, array $record, ?string $rkey = null, ?bool $validate = null, ?string $swapCommit = null) {
            expect($collection)->toBe('app.skyreader.feed.subscription');
            expect($record['$type'])->toBe('app.skyreader.feed.subscription');
            expect($record['feedUrl'])->toBe('https://example.com/rss.xml');
            expect($record['title'])->toBe('Example Blog');
            expect($record['sourceType'])->toBe('rss');
            expect($record)->not->toHaveKey('category');
            expect($validate)->toBeNull();
            expect($record['createdAt'])->toMatch('/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$/');

            return true;
        })
        ->andReturn($this->fakeSuccessResponse([
            'uri' => 'at://did:plc:test/app.skyreader.feed.subscription/abc',
            'cid' => 'bafy...',
        ]));

    $this->actingAs(User::factory()->create());

    $this->post(route('subscriptions.store'), [
        'subscriptions' => [[
            'feed_url' => 'https://example.com/rss.xml',
            'title' => 'Example Blog',
            'site_url' => 'https://example.com',
            'source_type' => 'rss',
        ]],
    ])->assertRedirect();
});

test('storing rejects a feed_url the user is already subscribed to', function () {
    $client = $this->fakeBlueskyClient();
    $this->fakeListRecords($client, ['https://example.com/rss.xml']);
    $client->shouldNotReceive('createRecord');

    $this->actingAs(User::factory()->create());

    $this->from(route('consume'))->post(route('subscriptions.store'), [
        'subscriptions' => [[
            'feed_url' => 'https://example.com/rss.xml',
            'title' => 'Example Blog',
            'site_url' => 'https://example.com',
            'source_type' => 'rss',
        ]],
    ])
        ->assertRedirect(route('consume'))
        ->assertSessionHasErrors('subscriptions.0.feed_url');
});

test('storing creates multiple subscriptions in one request', function () {
    $client = $this->fakeBlueskyClient();
    $this->fakeListRecords($client, []);
    $client->shouldReceive('createRecord')
        ->twice()
        ->andReturn(
            $this->fakeSuccessResponse(['uri' => 'at://did:plc:test/app.skyreader.feed.subscription/a', 'cid' => 'bafy1']),
            $this->fakeSuccessResponse(['uri' => 'at://did:plc:test/app.skyreader.feed.subscription/b', 'cid' => 'bafy2']),
        );

    $this->actingAs(User::factory()->create());

    $this->post(route('subscriptions.store'), [
        'subscriptions' => [
            [
                'feed_url' => 'https://example.com/rss.xml',
                'title' => 'Main',
                'site_url' => 'https://example.com',
                'source_type' => 'rss',
            ],
            [
                'feed_url' => 'https://example.com/comments.atom',
                'title' => 'Comments',
                'site_url' => 'https://example.com',
                'source_type' => 'rss',
            ],
        ],
    ])->assertRedirect()
        ->assertSessionHas('inertia.flash_data.toast.message', fn (string $msg) => str_contains($msg, '2 sources'));
});

test('storing succeeds the rest when one createRecord fails', function () {
    $client = $this->fakeBlueskyClient();
    $this->fakeListRecords($client, []);
    $client->shouldReceive('createRecord')
        ->twice()
        ->andReturn(
            $this->fakeSuccessResponse(['uri' => 'at://did:plc:test/app.skyreader.feed.subscription/a', 'cid' => 'bafy1']),
            $this->fakeFailureResponse(500, ['error' => 'InternalServerError']),
        );

    $this->actingAs(User::factory()->create());

    $this->post(route('subscriptions.store'), [
        'subscriptions' => [
            [
                'feed_url' => 'https://example.com/rss.xml',
                'title' => 'Main',
                'site_url' => 'https://example.com',
                'source_type' => 'rss',
            ],
            [
                'feed_url' => 'https://example.com/comments.atom',
                'title' => 'Comments',
                'site_url' => 'https://example.com',
                'source_type' => 'rss',
            ],
        ],
    ])->assertRedirect()
        ->assertSessionHas('inertia.flash_data.toast.message', fn (string $msg) => str_contains($msg, 'Subscribed to Main')
            && str_contains($msg, 'Failed: Comments'));
});

test('storing a subscription validates the source type', function () {
    $this->actingAs(freshenBluesky(User::factory()->create()));

    $this->from(route('consume'))->post(route('subscriptions.store'), [
        'subscriptions' => [[
            'feed_url' => 'https://example.com/rss.xml',
            'source_type' => 'newsletter',
        ]],
    ])
        ->assertRedirect(route('consume'))
        ->assertSessionHasErrors('subscriptions.0.source_type');
});

test('discover resolves YouTube channels to their videos.xml feed', function (string $fixture, string $url) {
    Http::fake([
        '*youtube.com*' => Http::response(
            file_get_contents(base_path("tests/Fixtures/youtube/{$fixture}")),
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    $this->fakeListRecords($this->fakeBlueskyClient(), []);

    $this->actingAs(User::factory()->create());

    $this->postJson(route('subscriptions.discover'), ['url' => $url])
        ->assertOk()
        ->assertJsonCount(1, 'candidates')
        ->assertJsonPath('candidates.0.feed_url', 'https://www.youtube.com/feeds/videos.xml?channel_id=UCBJycsmduvYEL83R_U4JriQ')
        ->assertJsonPath('candidates.0.source_type', 'video')
        ->assertJsonPath('candidates.0.title', 'Marques Brownlee');
})->with([
    'channel URL' => ['channel.html', 'https://www.youtube.com/channel/UCBJycsmduvYEL83R_U4JriQ'],
    '@handle URL' => ['handle.html', 'https://www.youtube.com/@mkbhd'],
    '/c/ vanity URL' => ['vanity.html', 'https://www.youtube.com/c/MarquesBrownlee'],
    '/user/ legacy URL' => ['legacy.html', 'https://www.youtube.com/user/marquesbrownlee'],
    '@handle URL with sidebar channels' => ['handle_with_sidebar.html', 'https://www.youtube.com/@mkbhd'],
]);

test('discover bypasses YouTube EU consent by sending the SOCS cookie', function () {
    Http::fake([
        '*youtube.com*' => Http::response(
            file_get_contents(base_path('tests/Fixtures/youtube/handle.html')),
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    $this->fakeListRecords($this->fakeBlueskyClient(), []);

    $this->actingAs(User::factory()->create());

    $this->postJson(route('subscriptions.discover'), [
        'url' => 'https://www.youtube.com/@mkbhd',
    ])->assertOk();

    Http::assertSent(fn ($request) => $request->header('Cookie') === ['SOCS=CAI']);
});

test('discover triggers a refresh when no bluesky_session is cached', function () {
    Http::fake([
        'example.com' => Http::response(
            '<html><head><link rel="alternate" type="application/rss+xml" title="Main" href="/rss.xml"></head></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    $user = User::factory()->create(['refresh_token' => 'valid-db-refresh-token']);

    $factory = blueskyFactoryMock();
    $factory->shouldReceive('refreshSession')
        ->once()
        ->andReturnUsing(function () use ($factory, $user) {
            // Simulate the package's real refresh: seed the session so subsequent
            // tokenForBluesky() calls in the same request layer the fresh access
            // token onto the DB-backed refresh_token instead of refreshing again.
            session()->put('bluesky_session', [
                'did' => $user->did,
                'access_token' => freshOAuthJwt(),
            ]);

            return $factory;
        });
    $this->fakeListRecordsOnFactory($factory, []);

    $this->actingAs($user);

    expect(session('bluesky_session'))->toBeNull();

    $this->postJson(route('subscriptions.discover'), ['url' => 'https://example.com'])
        ->assertOk();
});

test('a refresh failure during discover (inertia) logs the user out and redirects to login', function () {
    Http::fake([
        'example.com' => Http::response(
            '<html><head><link rel="alternate" type="application/rss+xml" title="Main" href="/rss.xml"></head></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    $client = $this->fakeBlueskyClient();
    $client->shouldReceive('listRecords')->andThrow(new AuthenticationException);

    $this->actingAs(User::factory()->create(['refresh_token' => 'invalid']));

    $this->postJson(
        route('subscriptions.discover'),
        ['url' => 'https://example.com'],
        ['X-Inertia' => 'true'],
    )
        ->assertStatus(409)
        ->assertHeader('X-Inertia-Location', route('login'));

    expect(Auth::check())->toBeFalse();
});

test('a refresh failure during discover (raw fetch) returns 401 json', function () {
    Http::fake([
        'example.com' => Http::response(
            '<html><head><link rel="alternate" type="application/rss+xml" title="Main" href="/rss.xml"></head></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    $client = $this->fakeBlueskyClient();
    $client->shouldReceive('listRecords')->andThrow(new AuthenticationException);

    $this->actingAs(User::factory()->create(['refresh_token' => 'invalid']));

    $this->postJson(route('subscriptions.discover'), ['url' => 'https://example.com'])
        ->assertUnauthorized()
        ->assertExactJson(['message' => 'Session expired']);

    expect(Auth::check())->toBeFalse();
});

test('a failure thrown directly by refreshSession also logs the user out (inertia)', function () {
    Http::fake([
        'example.com' => Http::response(
            '<html><head><link rel="alternate" type="application/rss+xml" title="Main" href="/rss.xml"></head></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    $this->fakeBlueskyRefreshFailure();

    $this->actingAs(User::factory()->create(['refresh_token' => 'invalid']));

    $this->postJson(
        route('subscriptions.discover'),
        ['url' => 'https://example.com'],
        ['X-Inertia' => 'true'],
    )
        ->assertStatus(409)
        ->assertHeader('X-Inertia-Location', route('login'));

    expect(Auth::check())->toBeFalse();
});

test('a failure thrown directly by refreshSession also logs the user out (raw fetch)', function () {
    Http::fake([
        'example.com' => Http::response(
            '<html><head><link rel="alternate" type="application/rss+xml" title="Main" href="/rss.xml"></head></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    $this->fakeBlueskyRefreshFailure();

    $this->actingAs(User::factory()->create(['refresh_token' => 'invalid']));

    $this->postJson(route('subscriptions.discover'), ['url' => 'https://example.com'])
        ->assertUnauthorized()
        ->assertExactJson(['message' => 'Session expired']);

    expect(Auth::check())->toBeFalse();
});

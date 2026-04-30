<?php

use App\Models\User;
use Illuminate\Http\Client\Response as HttpResponse;
use Illuminate\Support\Facades\Http;
use Mockery\MockInterface;
use Revolution\Bluesky\Client\AtpClient;
use Revolution\Bluesky\Contracts\Factory as BlueskyFactory;

beforeEach(function () {
    Http::preventStrayRequests();
});

test('guests cannot discover or store subscriptions', function () {
    $this->postJson(route('subscriptions.discover'), ['url' => 'https://example.com'])
        ->assertUnauthorized();

    $this->post(route('subscriptions.store'), [
        'feed_url' => 'https://example.com/rss.xml',
        'source_type' => 'rss',
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

    $this->actingAs(User::factory()->create());

    $response = $this->postJson(route('subscriptions.discover'), [
        'url' => 'https://example.com',
    ])->assertOk();

    $response->assertExactJson([
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
    ]);
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

    $this->actingAs(User::factory()->create());

    $this->postJson(route('subscriptions.discover'), ['url' => 'https://example.com'])
        ->assertOk()
        ->assertJsonPath('candidates.0.title', 'Example Blog')
        ->assertJsonPath('candidates.0.feed_url', 'https://example.com/rss.xml');
});

test('discover returns a 422 with a url error when the page exposes no feed', function () {
    Http::fake([
        'example.com' => Http::response(
            '<html><head><title>Plain page</title></head><body>Nothing here.</body></html>',
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

    $this->actingAs(User::factory()->create());

    $this->postJson(route('subscriptions.discover'), ['url' => 'https://example.com'])
        ->assertStatus(422)
        ->assertJsonValidationErrors('url');
});

test('discover rejects a malformed URL', function () {
    $this->actingAs(User::factory()->create());

    $this->postJson(route('subscriptions.discover'), ['url' => 'not-a-url'])
        ->assertStatus(422)
        ->assertJsonValidationErrors('url');
});

test('storing a subscription writes the chosen feed to the user PDS', function () {
    $client = Mockery::mock(AtpClient::class);
    $client->shouldReceive('createRecord')
        ->once()
        ->withArgs(function (string $repo, string $collection, array $record) {
            expect($collection)->toBe('app.skyreader.feed.subscription');
            expect($record['feedUrl'])->toBe('https://example.com/rss.xml');
            expect($record['title'])->toBe('Example Blog');
            expect($record['sourceType'])->toBe('rss');
            expect($record)->not->toHaveKey('category');

            return true;
        })
        ->andReturn(fakeSuccessResponse([
            'uri' => 'at://did:plc:test/app.skyreader.feed.subscription/abc',
            'cid' => 'bafy...',
        ]));

    bindBlueskyFactory($client);

    $user = User::factory()->create();
    $this->actingAs($user);

    $this->post(route('subscriptions.store'), [
        'feed_url' => 'https://example.com/rss.xml',
        'title' => 'Example Blog',
        'site_url' => 'https://example.com',
        'source_type' => 'rss',
    ])->assertRedirect();
});

test('storing a subscription validates the source type', function () {
    $this->actingAs(User::factory()->create());

    $this->from(route('consume'))->post(route('subscriptions.store'), [
        'feed_url' => 'https://example.com/rss.xml',
        'source_type' => 'newsletter',
    ])
        ->assertRedirect(route('consume'))
        ->assertSessionHasErrors('source_type');
});

test('discover resolves YouTube channels to their videos.xml feed', function (string $fixture, string $url) {
    Http::fake([
        '*youtube.com*' => Http::response(
            file_get_contents(base_path("tests/Fixtures/youtube/{$fixture}")),
            200,
            ['Content-Type' => 'text/html'],
        ),
    ]);

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

    $this->actingAs(User::factory()->create());

    $this->postJson(route('subscriptions.discover'), [
        'url' => 'https://www.youtube.com/@mkbhd',
    ])->assertOk();

    Http::assertSent(fn ($request) => $request->header('Cookie') === ['SOCS=CAI']);
});

function bindBlueskyFactory(MockInterface $client): void
{
    $factory = Mockery::mock(BlueskyFactory::class);
    $factory->shouldReceive('withToken')->andReturnSelf();
    $factory->shouldReceive('client')->with(true)->andReturn($client);

    app()->instance(BlueskyFactory::class, $factory);
}

function fakeSuccessResponse(array $body): HttpResponse
{
    return new HttpResponse(Http::response($body, 200)->wait());
}

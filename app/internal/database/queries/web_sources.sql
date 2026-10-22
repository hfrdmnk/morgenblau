-- name: CreateWebSource :exec
INSERT INTO web_sources (
    id, listing_key, listing_url, site_url, title, recipe, health, health_reason,
    setup_cost, created_by_did, created_at, updated_at, setup_at, setup_health, setup_health_reason,
    setup_tried_at, next_poll_at, anchored_by_did, anchored_at, offered_at
) VALUES (
    sqlc.arg(id), sqlc.arg(listing_key), sqlc.arg(listing_url), sqlc.arg(site_url), sqlc.arg(title),
    sqlc.arg(recipe), sqlc.arg(health), sqlc.arg(health_reason),
    sqlc.arg(setup_cost), sqlc.arg(created_by_did), sqlc.arg(created_at), sqlc.arg(created_at), sqlc.arg(created_at),
    sqlc.arg(health), sqlc.arg(health_reason), sqlc.arg(created_at), sqlc.narg(next_poll_at),
    sqlc.narg(anchored_by_did), sqlc.narg(anchored_at), sqlc.arg(created_at)
);

-- name: GetWebSource :one
SELECT * FROM web_sources WHERE id = ?;

-- name: GetWebSourceByKey :one
-- A listing key finds the source keyed by it, else the source it is an alias of.
SELECT ws.* FROM web_sources ws
LEFT JOIN web_source_aliases a ON a.source_id = ws.id AND a.listing_key = sqlc.arg(listing_key)
WHERE ws.listing_key = sqlc.arg(listing_key) OR a.listing_key IS NOT NULL
ORDER BY a.listing_key IS NOT NULL
LIMIT 1;

-- name: InsertWebSourceAlias :exec
INSERT INTO web_source_aliases (listing_key, source_id) VALUES (sqlc.arg(listing_key), sqlc.arg(source_id))
ON CONFLICT (listing_key) DO NOTHING;

-- name: DeleteWebSourceAlias :exec
DELETE FROM web_source_aliases WHERE listing_key = sqlc.arg(listing_key) AND source_id = sqlc.arg(source_id);

-- name: RekeyWebSource :exec
UPDATE web_sources SET listing_key = sqlc.arg(listing_key) WHERE id = sqlc.arg(id);

-- name: MarkWebSourceSectionChecked :execrows
-- Matches nothing when a setup replaced the source while the check ran; that setup answers for itself.
UPDATE web_sources SET
    recipe = json_set(recipe, '$.setup.section_checked', json('true')),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND setup_seq = sqlc.arg(setup_seq);

-- name: ReplaceWebSourceSetup :exec
UPDATE web_sources SET
    listing_url = sqlc.arg(listing_url),
    site_url = sqlc.arg(site_url),
    title = sqlc.arg(title),
    recipe = sqlc.arg(recipe),
    health = sqlc.arg(health),
    health_reason = sqlc.arg(health_reason),
    setup_cost = sqlc.arg(setup_cost),
    updated_at = sqlc.arg(updated_at),
    setup_at = sqlc.arg(updated_at),
    setup_health = sqlc.arg(health),
    setup_health_reason = sqlc.arg(health_reason),
    setup_seq = setup_seq + 1,
    setup_tried_at = sqlc.arg(updated_at),
    next_poll_at = sqlc.narg(next_poll_at),
    last_polled_at = NULL,
    etag = NULL,
    last_modified = NULL,
    link_set_hash = NULL,
    consecutive_failures = 0,
    last_checked_at = NULL,
    anchored_by_did = sqlc.narg(anchored_by_did),
    anchored_at = sqlc.narg(anchored_at),
    offered_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);

-- name: OfferWebSourceByListingKey :one
-- Finding the source and stamping it are one statement: a reader shown it may follow it any time now, so the sweep
-- must never delete it between the lookup and the stamp.
-- A key a source moved away from finds it through its alias; a source's own key wins over another source's alias.
UPDATE web_sources SET offered_at = sqlc.arg(offered_at)
WHERE id = COALESCE(
    (SELECT own.id FROM web_sources own WHERE own.listing_key = sqlc.arg(listing_key)),
    (SELECT a.source_id FROM web_source_aliases a WHERE a.listing_key = sqlc.arg(listing_key))
)
RETURNING *;

-- name: MarkWebSourceServed :exec
UPDATE web_sources SET served_at = sqlc.arg(served_at) WHERE id = sqlc.arg(id);

-- name: MarkWebSourceSetupTried :exec
UPDATE web_sources SET setup_tried_at = sqlc.arg(tried_at) WHERE id = sqlc.arg(id);

-- name: UpdateWebSourcePoll :execrows
-- Matches nothing when a setup replaced the source while the poll ran, so the poll's writes are dropped.
UPDATE web_sources SET
    recipe = sqlc.arg(recipe),
    health = sqlc.arg(health),
    health_reason = sqlc.arg(health_reason),
    last_polled_at = sqlc.arg(last_polled_at),
    next_poll_at = sqlc.arg(next_poll_at),
    etag = sqlc.narg(etag),
    last_modified = sqlc.narg(last_modified),
    link_set_hash = sqlc.narg(link_set_hash),
    consecutive_failures = sqlc.arg(consecutive_failures),
    last_checked_at = COALESCE(sqlc.narg(last_checked_at), last_checked_at),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id) AND setup_seq = sqlc.arg(setup_seq);

-- name: ListDueWebSources :many
-- A web source counts as followed while a reader subscribes to either of its feed URLs or an outside feed reader fetched it lately.
-- A setup the site refused has nothing to poll; an unsure one is polled, since the reader chose it from its preview.
SELECT ws.* FROM web_sources ws
WHERE ws.setup_health IN ('ok', 'needs_attention')
  AND ws.next_poll_at IS NOT NULL
  AND ws.next_poll_at <= sqlc.arg(now)
  AND (EXISTS (SELECT 1 FROM user_subscriptions us WHERE us.feed_url IN (sqlc.arg(feed_url_prefix) || ws.id, sqlc.arg(legacy_feed_url_prefix) || ws.id))
       OR ws.served_at > sqlc.arg(served_since))
ORDER BY ws.next_poll_at, ws.id
LIMIT sqlc.arg(limit);

-- name: IsWebSourceFollowedByOthers :one
-- Answers yes or no and never names a follower; an anchored rebuild of a feed other readers follow must be confirmed first.
SELECT EXISTS (
    SELECT 1 FROM user_subscriptions us WHERE us.feed_url IN (sqlc.arg(feed_url), sqlc.arg(legacy_feed_url)) AND us.did <> sqlc.arg(did)
) AS followed;

-- name: InsertWebSourceItem :exec
INSERT INTO web_source_items (source_id, url, title, published_at, excerpt, first_seen_at, history, position)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (source_id, url) DO NOTHING;

-- name: ListWebSourceKnownItems :many
SELECT url, title, published_at FROM web_source_items WHERE source_id = ?;

-- name: GetOldestWebSourceItemDate :one
SELECT published_at FROM web_source_items
WHERE source_id = ? AND published_at IS NOT NULL
ORDER BY published_at ASC
LIMIT 1;

-- name: CountWebSourceItems :one
SELECT COUNT(*) FROM web_source_items WHERE source_id = ?;

-- name: ListWebSourceFeedItems :many
-- Newest first: an item's own date, else the time a later poll first saw it; undated history keeps page order at the end.
SELECT url, title, published_at, excerpt, first_seen_at, history, position
FROM web_source_items
WHERE source_id = sqlc.arg(source_id)
ORDER BY COALESCE(published_at, CASE WHEN history = 0 THEN first_seen_at END) IS NULL,
         COALESCE(published_at, CASE WHEN history = 0 THEN first_seen_at END) DESC,
         first_seen_at DESC,
         position ASC
LIMIT sqlc.arg(limit);

-- name: DeleteWebSourcePolls :exec
DELETE FROM web_source_polls WHERE source_id = ?;

-- name: InsertWebSourcePoll :exec
INSERT INTO web_source_polls (source_id, polled_at, item_count, urls, titles, url_template, date_frac, title_shape)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListWebSourcePolls :many
SELECT * FROM web_source_polls
WHERE source_id = sqlc.arg(source_id)
ORDER BY id DESC
LIMIT sqlc.arg(limit);

-- name: PruneWebSourcePolls :exec
DELETE FROM web_source_polls
WHERE web_source_polls.source_id = sqlc.arg(source_id)
  AND web_source_polls.id NOT IN (
    SELECT kept.id FROM web_source_polls kept WHERE kept.source_id = sqlc.arg(source_id) ORDER BY kept.id DESC LIMIT sqlc.arg(keep)
  );

-- name: InsertWebSourceRepair :exec
INSERT INTO web_source_repairs (source_id, attempted_at, cause, outcome, step, rule, reason, used_model, usd)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetLastModelRepairAt :one
SELECT attempted_at FROM web_source_repairs
WHERE source_id = ? AND used_model = 1
ORDER BY attempted_at DESC
LIMIT 1;

-- name: CountModelRepairsSince :one
SELECT COUNT(*) FROM web_source_repairs
WHERE source_id = sqlc.arg(source_id) AND used_model = 1 AND attempted_at >= sqlc.arg(since);

-- name: PruneWebSourceRepairs :exec
DELETE FROM web_source_repairs WHERE source_id = sqlc.arg(source_id) AND attempted_at < sqlc.arg(before);

-- name: GetLastWeeklyRepair :one
SELECT attempted_at, outcome FROM web_source_repairs
WHERE source_id = sqlc.arg(source_id) AND cause = 'weekly_check'
ORDER BY id DESC
LIMIT 1;

-- name: ListWebSourcesCreatedSince :many
SELECT created_at FROM web_sources
WHERE created_by_did = sqlc.arg(did) AND created_at > sqlc.arg(since)
ORDER BY created_at
LIMIT sqlc.arg(limit);

-- name: InsertModelSpend :exec
INSERT INTO web_model_spend (spent_at, kind, usd) VALUES (?, ?, ?);

-- name: SumModelSpendSince :one
SELECT CAST(COALESCE(SUM(usd), 0) AS REAL) AS usd FROM web_model_spend WHERE spent_at >= sqlc.arg(since);

-- name: PruneModelSpend :exec
DELETE FROM web_model_spend WHERE spent_at < sqlc.arg(before);

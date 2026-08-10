import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { resolve } from "node:path"

const root = resolve(import.meta.dirname, "../..")
const noticeSource = readFileSync(
  resolve(root, "web/components/topic/new-topic-notice.tsx"),
  "utf8"
)
const headerSource = readFileSync(
  resolve(root, "web/components/layout/site-header.tsx"),
  "utf8"
)
const feedTabsSource = readFileSync(
  resolve(root, "web/components/topic/topic-feed-tabs.tsx"),
  "utf8"
)
const handlerSource = readFileSync(
  resolve(root, "internal/handlers/api/topic_handlers.go"),
  "utf8"
)
const routerSource = readFileSync(
  resolve(root, "internal/server/router.go"),
  "utf8"
)
const serviceSource = readFileSync(
  resolve(root, "internal/services/topic_service.go"),
  "utf8"
)
const visibleEventRepositorySource = readFileSync(
  resolve(
    root,
    "internal/repositories/topic_visible_event_repository.go"
  ),
  "utf8"
)
const publishServiceSource = readFileSync(
  resolve(root, "internal/services/topic_publish_service.go"),
  "utf8"
)
const migrationSource = readFileSync(
  resolve(root, "migrations/migration.go"),
  "utf8"
)

assert.match(
  routerSource,
  /GET\("\/new_status",\s*apiHandlers\.TopicNewStatus\)/,
  "The new-topic status endpoint should be registered"
)
assert.match(
  serviceSource,
  /TopicVisibleEventRepository\.GetRoleStatus\(sqls\.DB\(\),\s*userId,\s*roleName,\s*after\)/,
  "New-topic status should use the persistent visibility-event cursor"
)
assert.match(
  visibleEventRepositorySource,
  /Scan\(&result\)\.Error[\s\S]*?return 0, 0, err/,
  "Visibility status query errors should propagate instead of returning successful zero values"
)
assert.match(
  handlerSource,
  /GetNewTopicStatus\([\s\S]*?statusErr[\s\S]*?ginx\.WriteJSON\(ctx, statusErr\)[\s\S]*?return/,
  "The API should return a failure envelope when a role status query fails"
)
assert.match(
  visibleEventRepositorySource,
  /COALESCE\(MAX\(visible_event\.id\),\s*0\)[\s\S]*?COUNT\(DISTINCT CASE[\s\S]*?visible_event\.id > \?[\s\S]*?topic\.user_id <> \?[\s\S]*?t_role\.name = \?/,
  "Marker and distinct-topic count should be computed in one role-filtered query"
)
assert.match(
  publishServiceSource,
  /topic\.Status == constants\.StatusOk[\s\S]*?createTopicVisibleEventTx\(ctx\.Tx,\s*topic\.Id\)/,
  "Publishing a visible topic should create its visibility event in the publish transaction"
)
assert.equal(
  migrationSource.includes(
    'register(17, "backfill topic visible events", migrate_topic_visible_events)'
  ),
  true,
  "The visibility-event backfill migration should be registered"
)
for (const expected of ['"agentAfter"', '"userAfter"', '"roles": roles']) {
  assert.equal(
    handlerSource.includes(expected),
    true,
    `The status response should contain ${expected}`
  )
}
assert.match(
  handlerSource,
  /\{roleName:\s*"agent"[\s\S]*?\{roleName:\s*"用户"/,
  "The status endpoint should return agent and user roles in one response"
)
assert.match(
  noticeSource,
  /NEW_TOPIC_POLL_INTERVAL_MS\s*=\s*30_000/,
  "New topics should be polled every 30 seconds"
)
for (const expected of [
  '"/api/topic/new_status"',
  '"visibilitychange"',
  '"focus"',
  '"storage"',
  '"bbsgo.new-topic-marker"',
  'params: { agentAfter, userAfter }',
  "router.push(target)",
  "router.refresh()",
]) {
  assert.equal(
    noticeSource.includes(expected),
    true,
    `NewTopicNotice should contain ${expected}`
  )
}
assert.equal(
  noticeSource.includes("markerStorageKey(userId, roleName)"),
  true,
  "Each user and role should have an independent local marker"
)
assert.match(
  noticeSource,
  /function normalizeMarker[\s\S]*?\^\\d\+\$[\s\S]*?catch \{[\s\S]*?transient polling failure[\s\S]*?\} finally/,
  'Marker "0" should remain valid while polling failures leave stored markers unchanged'
)
assert.match(
  noticeSource,
  /activeRequestRef[\s\S]*?generation[\s\S]*?inFlightRequestRef[\s\S]*?activeRequestRef\.current\.generation !== request\.generation/,
  "Polling requests should be generation-bound so an old user cannot block or update a new user"
)
assert.equal(
  headerSource.includes("NewTopicNoticeButton") ||
    headerSource.includes("useNewTopicNotice"),
  false,
  "SiteHeader should no longer render a global new-topic button"
)
assert.equal(
  feedTabsSource.includes("useRoleNewTopicNotices(user?.id)"),
  true,
  "TopicFeedTabs should own the role-specific polling state"
)
assert.match(
  feedTabsSource,
  /count\s*>\s*0[\s\S]*?rounded-full bg-sky-500/,
  "Role filters should show a stable blue dot when new topics exist"
)
assert.match(
  serviceSource,
  /Where\("id = \? AND status <> \?", id, constants\.StatusOk\)[\s\S]*?RowsAffected == 0[\s\S]*?createTopicVisibleEventTx/,
  "Audit should atomically create an event only for an actual visibility transition"
)
assert.match(
  serviceSource,
  /Where\("id = \? AND status = \?", id, constants\.StatusDeleted\)[\s\S]*?RowsAffected == 0[\s\S]*?UndeleteByTopicId[\s\S]*?createTopicVisibleEventTx/,
  "Undelete should atomically restore only deleted topics and emit one event"
)
assert.match(
  feedTabsSource,
  /event\.button !== 0[\s\S]*?event\.metaKey[\s\S]*?event\.ctrlKey[\s\S]*?event\.shiftKey[\s\S]*?event\.altKey[\s\S]*?return[\s\S]*?event\.preventDefault\(\)[\s\S]*?openRoleTopics\(item\.roleName\)/,
  "Only an unmodified left click should be intercepted; modified clicks keep native link behavior"
)
assert.equal(
  headerSource.includes("<MsgNotice count={unreadMessageCount} />"),
  true,
  "The existing private-message bell should remain separate"
)

console.log("new topic notice behavior is covered")

# EduExchange — System Design

## Overview

EduExchange is an educational resource marketplace where authors publish learning materials, reviewers moderate quality, suppliers manage physical orders, and users discover content through search and recommendations.

**Stack:** Go 1.22, Gin, Templ v0.2.778, HTMX, Tailwind CSS, PostgreSQL 16, pgx/v5, Docker

---

## Architecture

### Three-Layer Design

```
HTTP Handler  →  Service  →  Repository
    (Gin)      (business)   (SQL / pgx)
```

- **Handlers** parse HTTP, call services, render Templ pages or JSON
- **Services** enforce business rules, orchestrate repositories, fire notifications
- **Repositories** execute SQL via `pgxpool`; no ORM

### Package Layout

```
cmd/
  server/          — main entry point
  seed/            — development seed data
internal/
  audit/           — append-only audit log service
  cron/            — scheduled jobs (retry processor, analytics refresh)
  handler/         — HTTP handlers per domain
  middleware/      — auth, ban check, rate limit, role guard
  model/           — domain types and enums
  repository/      — DB interfaces + postgres implementations
  service/         — business logic
  sse/             — SSE hub for real-time push
  templ/           — Templ components and page templates
migrations/        — golang-migrate up/down SQL files
tests/
  frontend/        — Templ render tests (no DB)
  integration/     — end-to-end HTTP tests (requires Docker DB)
docs/              — this directory
```

---

## Domain Models and State Machines

### Resource Lifecycle

```
DRAFT ──► PENDING_REVIEW ──► APPROVED ──► PUBLISHED
  ▲              │                              │
  └── REJECTED ◄─┘                        TAKEN_DOWN
```

- Author creates DRAFT, edits, submits for review
- Reviewer approves or rejects
- Admin publishes approved resources
- Reviewer or admin can take down published resources
- Admin can restore taken-down resources
- Author can revise rejected resources (→ DRAFT)
- Published resources can be re-edited (creates new version, → PENDING_REVIEW)

### Optimistic Locking

Every `UPDATE resources` checks `version = $expected_version`. Stale edits return 409.

### Supplier Order Lifecycle

```
OPEN ──► CONFIRMED ──► SHIPPED ──► RECEIVED ──► QC_PASS/QC_FAIL ──► CLOSED
  └─────────────────────────────────────────────────────────────────► CANCELLED
```

---

## Search

### Full-Text Search (PostgreSQL)

Resources are indexed in `search_index` with:
- `tsvector_content` — PostgreSQL tsvector for English stemming
- `pinyin_content` — transliterated content for Chinese title search
- `tag_content` — space-separated tag names

Queries use `websearch_to_tsquery` with `ts_rank_cd` for scoring.

### Pinyin Support

Chinese characters are transliterated to pinyin using `pinyin_mapping` table. Stored in `search_index.pinyin_content` with trigram index for fuzzy matching.

### Synonym Expansion

`synonym_groups` table holds arrays of equivalent terms. At query time, all synonyms for a matched term are added to the search query with OR semantics.

### Did-You-Mean

`pg_trgm` similarity on `search_terms.term` against the query string. Suggestions returned when similarity > 0.3.

---

## Gamification

### Points

Each engagement action earns points per configurable `point_rules`:

| Action | Default Points |
|--------|---------------|
| `resource_create` | 10 |
| `resource_publish` | 25 |
| `vote_received` | 2 |
| `favorite_received` | 3 |
| `comment_received` | 1 |

Point rules are admin-configurable at runtime.

### Levels

Thresholds defined in `gamification/service.go`:

| Level | Points Required |
|-------|----------------|
| 0 | 0 |
| 1 | 100 |
| 2 | 300 |
| 3 | 600 |
| 4 | 1000 |

Level-up fires a `level_up` SSE/DB notification.

### Badges

Awarded by `checkAndAwardBadges` on each point transaction:

| Badge | Condition |
|-------|-----------|
| `first_resource` | First resource published |
| `popular_author` | 50+ favorites received |
| `top_contributor` | 500+ points total |

Badge award fires a `badge_earned` SSE/DB notification.

---

## Recommendations

`RecommendationService` aggregates results from pluggable `RecommendationStrategy` implementations:

| Strategy | Logic |
|----------|-------|
| `MostEngagedCategories` | Returns resources from categories the user has favorited most |
| `FollowedAuthorNewContent` | Returns recently published resources by authors the user follows |
| `SimilarTagAffinity` | Returns resources sharing tags with the user's most-favorited resources |

Each strategy has a configurable weight stored in `recommendation_strategies` table. Results are merged, deduplicated, and capped at 20.

---

## Notification System

### Flow

```
Service action (approve/ban/publish)
  → NotificationSender.Send(userID, eventType, title, body, resourceID)
    → Check notification_subscriptions (skip if disabled)
    → INSERT into notifications
    → SSE push via Hub.SendToUser(userID, SSEEvent)
    → On SSE failure: INSERT into notification_retry_queue
```

### SSE Hub

`internal/sse/hub.go` maintains a `map[uuid.UUID][]chan SSEEvent` of registered clients. `Register` / `Unregister` use a mutex. `SendToUser` fans out to all sessions for a user.

### Retry Queue

`RetryService.ProcessRetryQueue()` runs every minute (cron):
- Fetches `PENDING` items with `next_retry_at <= NOW()`
- Attempts to create the notification
- On success: deletes the queue item
- On failure: increments `attempts`; if `attempts >= 5` → marks `FAILED`; else schedules next retry at `[1, 2, 4, 8, 15]` minutes

### Event Types

| Event | Trigger |
|-------|---------|
| `review_decision` | Resource approved or rejected |
| `publish_complete` | Resource published |
| `follow_new_content` | Author a user follows publishes |
| `badge_earned` | User earns a badge |
| `level_up` | User's level increases |
| `ban_notice` | User is banned |
| `supplier_shipment` | Supplier submits ASN |

---

## Analytics

### Dashboard

`AnalyticsService.GetDashboard` returns role-filtered `DashboardMetrics`:

| Role | Visible metrics |
|------|----------------|
| Admin | All: cycle time, violation rate, cancellation rate, totals, hotspots, peaks |
| Reviewer | Moderation: violation rate, report counts |
| Supplier | Own KPIs from `supplier_kpis` |
| User | Public: total resources, demand hotspots |

### Analytics Refresh

`AnalyticsService.RefreshMetrics` runs every 15 minutes (cron). It:
1. Computes demand hotspots (category engagement frequency)
2. Computes utilization peaks (hourly engagement distribution)
3. Upserts into `analytics_summary` with UNIQUE(metric_type, metric_key)

### Reports

`GenerateReport` writes a CSV to `data/exports/reports/` and creates a `ScheduledReport` record. Supported report types: `ANALYTICS`.

### Audit Log

All state-changing service operations call `audit.Service.Record(ctx, Entry)` which inserts into `audit_logs`. The audit log is exportable to CSV by admins.

---

## Like-Ring Detection

`EngagementService.PostVote` runs anomaly detection after each upvote:
1. Queries `GetMutualVoteCount(userA, userB, 24h)` 
2. If count ≥ 16: creates `AnomalyFlag` of type `LIKE_RING`
3. Suspends voting rights by setting `AnomalyFlag.status = OPEN`

Reviewers can review anomaly flags at `/moderation/anomalies`.

---

## Database Schema Overview

### Core Tables

| Table | Purpose |
|-------|---------|
| `users` | User accounts |
| `user_roles` | Role assignments (ADMIN, AUTHOR, REVIEWER, SUPPLIER) |
| `user_profiles` | Extended profile data |
| `sessions` | Session tokens (24h TTL) |
| `user_bans` | Active/expired bans |
| `audit_logs` | Immutable audit trail |
| `rate_limit_counters` | Per-user per-action sliding window counters |

### Catalog Tables

| Table | Purpose |
|-------|---------|
| `resources` | Resource records with optimistic lock version |
| `resource_versions` | Immutable snapshots of each edit |
| `resource_reviews` | Reviewer approve/reject decisions |
| `resource_files` | Attached file metadata |
| `tags` | Tag definitions |
| `resource_tags` | Many-to-many resource↔tag |
| `categories` | Hierarchical categories |
| `bulk_import_jobs` | CSV import job state |

### Engagement Tables

| Table | Purpose |
|-------|---------|
| `votes` | Up/down votes |
| `favorites` | User favorites |
| `follows` | Author follows |
| `anomaly_flags` | Like-ring and other anomaly records |

### Gamification Tables

| Table | Purpose |
|-------|---------|
| `user_points` | Cumulative points + level per user |
| `point_transactions` | Individual point award history |
| `user_badges` | Awarded badges |
| `point_rules` | Configurable point values per action |
| `ranking_archives` | Historical leaderboard snapshots |

### Search Tables

| Table | Purpose |
|-------|---------|
| `search_index` | tsvector + pinyin + tag search index |
| `search_terms` | Term frequency for suggestions |
| `user_search_history` | Per-user query history |
| `pinyin_mapping` | Chinese character → pinyin |
| `synonym_groups` | Synonym expansion arrays |
| `recommendation_strategies` | Strategy weights and enable flags |

### Supplier Tables

| Table | Purpose |
|-------|---------|
| `suppliers` | Supplier profiles and tier (GOLD/SILVER/BRONZE) |
| `supplier_orders` | Purchase orders |
| `supplier_asns` | Advance ship notices |
| `supplier_qc_results` | Quality control results |
| `supplier_kpis` | Calculated KPI metrics |

### Notification Tables

| Table | Purpose |
|-------|---------|
| `notifications` | Delivered notification records |
| `notification_subscriptions` | Per-user event opt-in/out |
| `notification_retry_queue` | Failed deliveries awaiting retry |

### Analytics Tables

| Table | Purpose |
|-------|---------|
| `analytics_summary` | Computed metric snapshots |
| `scheduled_reports` | Generated report records |

---

## Security

- **Authentication**: Session cookie (`session_token`). `middleware.AuthMiddleware` verifies token and sets `AuthUser` in Gin context.
- **Ban Check**: `BanCheckMiddleware` rejects banned users with 403 before any handler runs.
- **Role Guard**: `middleware.RequireRole(roles...)` returns 403 if user lacks the required role.
- **Rate Limiting**: `middleware.RateLimitMiddleware` uses `rate_limit_counters` with per-hour sliding windows. Exceeded → 429.
- **Input Validation**: All user inputs validated at service layer; `model.ValidationErrors` returns structured field-level errors.
- **File Validation**: MIME type detected from content (not extension). Only PDF, DOCX, PPTX, MP4 allowed. Max 50 MB, 5 files per resource.
- **Optimistic Locking**: Version mismatch on resource edit → 409.
- **SQL Injection**: All queries use parameterized `pgx` queries. No string interpolation in SQL.

---

## Scheduled Jobs (Cron)

| Schedule | Job | Description |
|----------|-----|-------------|
| Every 1 min | `NotificationRetryProcessor` | Processes failed notification deliveries |
| Every 15 min | `AnalyticsRefresh` | Refreshes analytics_summary metrics |
| Every hour | `RankingArchive` | Archives leaderboard snapshot |
| Every 6 hours | `KPIRecalculate` | Recalculates supplier KPI scores |

---

## Testing Strategy

### Frontend Tests (`tests/frontend/`)

- Unit tests for all Templ components using `context.Background()` and `bytes.Buffer`
- No database or network required
- Verify HTML output contains expected strings

### Integration Tests (`tests/integration/`)

- Full HTTP round-trip tests against a real PostgreSQL database
- Docker Compose provides `db:5432` for CI
- Each test calls `truncate(t)` to reset state
- Tests verify status codes, JSON response fields, and DB state via direct `testPool.QueryRow`

### Running Tests

```bash
# Frontend tests (no Docker needed)
go test ./tests/frontend/...

# Integration tests (requires Docker Compose with DB)
docker-compose up -d db
go test ./tests/integration/... -v -timeout 120s

# All unit tests (no external deps)
go test ./internal/...
```

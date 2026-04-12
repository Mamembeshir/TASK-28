# EduExchange — Design Decisions and Open Questions

This document records key design decisions made during implementation and open questions for future consideration.

---

## Resolved Design Decisions

### 1.1 Optimistic Locking on Resources

**Decision:** Integer `version` column on `resources`. Every update checks `WHERE id = $1 AND version = $2`. Stale reads return 409.

**Rationale:** Prevents lost updates on concurrent edits without a distributed lock. Version is included in every form as a hidden field.

### 1.2 EditPublished Flow

**Decision:** Editing a published resource transitions it to `PENDING_REVIEW` and creates a new version snapshot. Old published content remains visible until the re-edited version is approved and re-published.

**Rationale:** Authors can improve resources without taking them offline. Reviewers see the diff between versions.

### 1.3 Notification Sender Interface Pattern

**Decision:** Services that fire notifications (catalog, gamification, moderation, supplier) define their own `NotificationSender` interface locally. The concrete `NotificationService` satisfies all of them. Wiring happens in `main.go` via `SetNotificationSender()` after construction.

**Rationale:** Avoids import cycles — each service package stays independent of the messaging package.

### 1.4 SSE Hub + Retry Queue

**Decision:** Real-time delivery via SSE hub. Failed deliveries go to `notification_retry_queue` (processed every minute with exponential backoff: 1, 2, 4, 8, 15 minutes; max 5 attempts).

**Rationale:** If a user is offline when a notification fires, it persists in `notifications` (visible in messaging center) and the retry queue attempts re-delivery.

### 1.5 Role-Filtered Analytics Dashboard

**Decision:** `GET /analytics/dashboard` returns different data depending on caller's role. Single endpoint, role-switched response.

**Rationale:** Avoids N separate endpoints while keeping sensitive metrics out of regular user responses.

### 1.6 Pinyin Search via search_index

**Decision:** Chinese-title resources have their pinyin transliteration stored in `search_index.pinyin_content` at write time. Queries check both `tsvector_content` and `pinyin_content`.

**Rationale:** PostgreSQL's built-in full-text search doesn't support Chinese tokenization. Pre-computed pinyin allows fast fuzzy matching without an external search engine.

### 1.7 Supplier Notification via adminFinderFn

**Decision:** When a supplier submits an ASN, notifications fan out to all admin users. `SupplierService` receives an `adminFinderFn func(ctx) []uuid.UUID` callback passed via `SetNotificationSender`.

**Rationale:** Keeps `SupplierService` decoupled from the database pool.

### 1.8 Recommendation Strategy Pattern

**Decision:** `[]RecommendationStrategy` slice injected at startup. Each implements `Recommend(ctx, userID) ([]uuid.UUID, error)`. Weights stored in DB and admin-configurable.

**Rationale:** New strategies can be added without modifying existing code.

---

## Open Questions

### 2.1 Email Notifications

**Status:** Not implemented. The retry queue schema is compatible with adding email as a delivery channel (add `delivery_channel` column).

### 2.2 Multi-Tenancy / Organizations

**Status:** Not planned. All resources exist in a single namespace. Multi-tenancy would require an `org_id` on all tables.

### 2.3 Full Chinese Tokenization

**Status:** Partial (pinyin only). True Chinese full-text search would require `zhparser` (PostgreSQL extension) or an external search engine.

### 2.4 File Storage

**Status:** Local disk only (`data/uploads/`). Production requires an object store (S3, GCS, Azure Blob).

### 2.5 Audit Log Retention

**Status:** No TTL policy. `audit_logs` grows unboundedly. A periodic archival job would be needed for production.

### 2.6 Search Index Sync

**Status:** Synchronous at write time. Under high write load, async worker processing a write queue would decouple this.

### 2.7 Supplier KPI Tier Thresholds

**Status:** Hardcoded. Making them configurable (DB-stored, admin-editable) would support different SLAs per contract.

### 2.8 Rate Limit Persistence

**Status:** Uses `rate_limit_counters` table. Under very high traffic, moving to Redis would reduce DB pressure.

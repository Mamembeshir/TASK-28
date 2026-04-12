# EduExchange — Business Logic Questions Log

---

## 1. Catalog & Content

### 1.1 What is the difference between APPROVED and PUBLISHED?
* **Question:** The prompt mentions both review approval and publishing. Why two separate states?
* **My Understanding:** Approval means a reviewer has verified quality/compliance. Publishing means the author has chosen to make it visible in the catalog. An approved resource might not be published yet if the author wants to wait.
* **Solution:** Two-step: Reviewer approves (quality gate) → Author publishes (visibility gate). This gives authors control over timing while maintaining review compliance. Only PUBLISHED resources appear in search/browse for Regular Users.

### 1.2 Can an Author edit a PUBLISHED resource?
* **Question:** If an author edits a published resource, does it need re-review?
* **My Understanding:** Edits to published content should go through review again to maintain quality control. The edit creates a new version, and the resource goes back to PENDING_REVIEW while the current published version remains visible.
* **Solution:** Editing a PUBLISHED resource creates a new ResourceVersion and sets status → PENDING_REVIEW. The previous published version remains visible to users until the new version is approved and re-published. This prevents content disappearing during review.

### 1.3 What file types are allowed for resource uploads?
* **Question:** The prompt says "locally hosted learning resources" but doesn't specify formats.
* **My Understanding:** Educational resources are typically documents and images. Video would be too large for offline storage at scale.
* **Solution:** Allowed: PDF, DOCX, PPTX, XLSX, JPEG, PNG. Max 50 MB per file, max 5 files per resource. Validated by MIME type (not just extension). SHA-256 checksum for dedup per author.

---

## 2. Search & Pinyin

### 2.1 How does pinyin search work technically?
* **Question:** The prompt says "pinyin and synonym typo correction." How is pinyin matching implemented in PostgreSQL?
* **My Understanding:** A mapping table converts Chinese characters to pinyin. When indexing, both the original text and its pinyin representation are stored. When searching, the query is also converted to pinyin for matching.
* **Solution:** `PinyinMapping` table maps individual characters to pinyin strings. On resource create/update, a trigger populates `SearchIndex.pinyin_content` by converting all Chinese characters in title/description to pinyin. Search queries are also converted to pinyin before matching. This allows "jiaoxue" to match "教学" (jiāo xué). The conversion strips tones for broader matching.

### 2.2 How does typo correction work?
* **Question:** The prompt mentions "synonym typo correction." Are synonyms and typos the same thing?
* **My Understanding:** They're separate features. Typo correction uses fuzzy matching (e.g., "mathamatics" → "mathematics"). Synonym expansion matches related terms (e.g., "math" → "mathematics, arithmetic, calculus").
* **Solution:** Two separate mechanisms: (1) Typo correction via `pg_trgm` extension — compute `similarity(query, term)` against SearchTerm table. If similarity > 0.3 and no exact results, suggest "Did you mean: [corrected]?" (2) Synonym expansion via `SynonymGroup` table — if query matches any term in a group, expand search to include all terms in that group. Admin can manage synonym groups.

### 2.3 When do rankings reset and how?
* **Question:** The prompt says "bestseller and new release rankings that reset weekly every Monday at 2:00 AM." What happens to the old data?
* **My Understanding:** The previous week's rankings should be archived before reset so they can be referenced.
* **Solution:** Monday 02:00 AM job: (1) snapshot current week's rankings into a `RankingArchive` table with week number, (2) reset the counters/sorting criteria for the new week. Bestseller counts upvotes received in the current week only. New releases counts by publish date in the current week. Both are top-20 lists. Previous weeks' rankings are viewable via archive but not displayed on the home page.

---

## 3. Gamification

### 3.1 Can points go negative? What happens to level?
* **Question:** The prompt says -10 for takedown. If a new user with 3 points gets a takedown, they'd have -7 points.
* **My Understanding:** Points can go negative (that's the penalty's purpose), but level should floor at 0.
* **Solution:** `total_points` can be negative. `level = max(0, floor(total_points / 200))`. A user at -7 points is level 0. They need to earn 207 points to reach level 1. All point changes are recorded in `PointTransaction` for full audit trail.

### 3.2 Are badges ever revoked?
* **Question:** If a user earned "50 Favorites Received" badge but then some favorites are removed (unfavorited), do they lose the badge?
* **My Understanding:** Badges should be permanent once awarded. Revoking badges creates confusion and reduces motivation.
* **Solution:** Once awarded, never revoked. The threshold check happens at the moment of the qualifying event (e.g., receiving the 50th favorite). Even if favorites later decrease below 50, the badge remains. The `UserBadge.awarded_at` timestamp serves as proof.

### 3.3 How are point rules made configurable?
* **Question:** The prompt says "configurable rules" for points. How does an Admin change them?
* **My Understanding:** Admin UI to edit point values per event type. Changes apply going forward, not retroactively.
* **Solution:** `PointRule` table: event_type (enum), points (signed int), description, is_active. Admin can edit point values and enable/disable rules. Changes are audit-logged. New values apply to future events only — no retroactive recalculation. The gamification service looks up the current rule on each event.

---

## 4. Moderation

### 4.1 How does like-ring detection work exactly?
* **Question:** The prompt says "more than 15 mutual likes between the same two accounts in 24 hours." What constitutes a "mutual like"?
* **My Understanding:** User A upvotes User B's resources AND User B upvotes User A's resources. If the count of (A→B upvotes) + (B→A upvotes) exceeds 15 in 24 hours, it's flagged.
* **Solution:** Every 6 hours, a job queries: for each pair of users who have exchanged upvotes in the last 24h, count total mutual upvotes. If count > 15: create AnomalyFlag with both user IDs and evidence (list of vote IDs, timestamps). Both users' future votes are suspended (not deleted) until a Reviewer reviews the flag. The flag can be DISMISSED (false positive, votes restored) or REVIEWED (votes removed, warning/ban issued).

### 4.2 What does "rate limit 20 posts per hour" mean exactly?
* **Question:** Is it a sliding window or a fixed window?
* **My Understanding:** A sliding window is more accurate but harder to implement. A fixed window (per clock hour) is simpler and good enough for abuse prevention.
* **Solution:** Fixed window per clock hour. `RateLimitCounter` table: (user_id, action_type, window_start, count). Window_start is the current hour truncated (e.g., 14:00:00). On each post attempt: find or create counter for current hour. If count >= 20, reject with 429. Counter rows are cleaned up daily. This is simple, deterministic, and easy to audit.

### 4.3 What evidence is stored for moderation actions?
* **Question:** The prompt says "evidence chains" for moderation. What specifically?
* **My Understanding:** Every moderation decision needs supporting evidence so it can be reviewed or appealed.
* **Solution:** `ModerationAction.evidence_json` stores: the report(s) that triggered the action, the specific content that violated rules, screenshots/excerpts if applicable (as text references, not binary), the rule or policy violated, and any prior warnings. This JSON blob is immutable once created. Multiple ModerationActions can chain together (e.g., first warning → second warning → ban), linked via the user_id and timestamps.

---

## 5. Supplier Fulfillment

### 5.1 What happens when a supplier misses the 48-hour delivery confirmation window?
* **Question:** The prompt says delivery-date confirmation within 48 hours. What if they miss it?
* **My Understanding:** An escalation alert should be generated, not an automatic order cancellation.
* **Solution:** After 48 hours without confirmation: (1) create a notification for Admin (anomaly_alert type), (2) create an AnomalyFlag for the order, (3) the order stays in CREATED status. Admin decides to extend the window, reassign, or cancel. The missed deadline is recorded in the supplier's KPI data (affects OTD calculation).

### 5.2 How are KPI rolling windows calculated?
* **Question:** The prompt says nightly recalculation. Is it calendar months or rolling days?
* **My Understanding:** Rolling 90-day windows are more accurate and avoid month-boundary spikes.
* **Solution:** Rolling 90-day window from the calculation date. At 01:00 AM: for each supplier, query all orders in the last 90 days. Calculate: OTD = orders delivered on/before confirmed date / total delivered orders. Stockout = order lines that couldn't be fulfilled / total order lines. Return rate = returned units within 30 days of receipt / total received units. Defect rate = defective units per QC / total inspected. Store in `SupplierKPI` with period_start = today-90, period_end = today.

### 5.3 What does each supplier tier get?
* **Question:** The prompt says tiers "drive dashboard visibility and escalation thresholds." What specifically?
* **My Understanding:** Higher tiers get more favorable treatment — more lenient escalation windows and better dashboard placement.
* **Solution:** Gold: 72h delivery confirmation window (vs 48h default), featured on supplier dashboard, escalation only after 2 missed deadlines. Silver: standard 48h window, normal dashboard, escalation after 1 miss. Bronze: 48h window, lower dashboard position, immediate escalation on any miss. Tier also affects which orders appear first in Admin's review queue (Bronze orders are prioritized for oversight).

---

## 6. Notifications

### 6.1 How does the retry queue work with SSE?
* **Question:** The prompt says "offline retry queue that reattempts delivery to the user inbox up to 5 times over 30 minutes if a write fails." But SSE is a push mechanism — what "write" can fail?
* **My Understanding:** The retry isn't about SSE delivery — it's about persisting the notification record to the database. If the DB write fails (transaction conflict, connection issue), the system retries writing the notification. SSE pushes the notification to the browser after successful write.
* **Solution:** Flow: (1) Event occurs → NotificationService creates Notification record in DB. (2) If DB write fails → queue for retry (1, 2, 4, 8, 15 min intervals). (3) On successful DB write → push via SSE to connected user. (4) If user not connected via SSE → notification waits in DB, user sees it on next page load. (5) After 5 failed DB writes → FAILED status + anomaly alert to Admin. The retry handles DB resilience, SSE handles real-time delivery.

### 6.2 What event types exist for notifications?
* **Question:** The prompt lists some but may not be exhaustive.
* **My Understanding:** Need to cover all workflow state changes that a user would care about.
* **Solution:** Event types: `entry_deadline` (review deadline approaching), `review_decision` (approved/rejected), `publish_complete` (resource now live), `supplier_shipment` (ASN received), `supplier_qc` (QC result), `anomaly_alert` (system flag), `ban_notice` (user banned), `report_update` (report resolved/dismissed), `badge_earned` (new badge), `level_up` (new level), `follow_new_content` (followed author published). All configurable per user via NotificationSubscription.

---

## 7. Analytics & Reporting

### 7.1 What does "permission-isolated" mean for analytics?
* **Question:** Different roles see different analytics. What exactly?
* **My Understanding:** Each role gets a dashboard scoped to their domain.
* **Solution:** Admin: full analytics (all metrics, all users, all suppliers). Reviewer/Moderator: moderation stats (reports volume, resolution time, violation rate). Supplier: own KPIs, own order performance. Author: own resource stats (views, votes, favorites). Regular User: public stats only (popular categories, trending resources). The analytics API filters results based on the authenticated user's role.

### 7.2 How are scheduled reports generated?
* **Question:** The prompt says "scheduled report generation to local files." What format and trigger?
* **My Understanding:** CSV files generated on a configurable schedule, saved to local disk.
* **Solution:** `ScheduledReport` table tracks report configurations. Admin sets: report type, parameters (date range, filters), schedule (cron expression). The cron job generates a CSV file at `data/exports/reports/{report_type}_{date}.csv`. Report generation itself is audit-logged. Files are accessible only to Admin via an authenticated download endpoint.
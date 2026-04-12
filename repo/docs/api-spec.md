# EduExchange — API Reference

All HTML endpoints accept cookies for session auth. JSON responses are returned when the client sends `Accept: application/json`. Form submissions use `application/x-www-form-urlencoded`. Redirects after mutations use `303 See Other`.

---

## Authentication

Session cookie: `session_token` (set on login, cleared on logout, 24 h TTL).

---

## 1. Auth (public)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/login` | Login form |
| POST | `/login` | Authenticate: `username`, `password` → sets cookie → 303 `/` |
| GET | `/register` | Register form |
| POST | `/register` | Create account: `username`, `email`, `password` → 303 `/login` |
| POST | `/logout` | Clear session → 303 `/login` |

---

## 2. Home & Search (authenticated)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/` | public | Home page — bestsellers, new releases, recommendations |
| GET | `/search` | auth | Full-text search: `?q=`, `?category_id=`, `?tag=`, `?page=`, `?page_size=` |
| GET | `/search/suggest` | public | Autocomplete: `?q=` → `{"suggestions":[...]}` |
| GET | `/search/history` | auth | User's recent search queries |
| DELETE | `/search/history` | auth | Clear search history |
| GET | `/rankings/bestsellers` | auth | Top-voted resources |
| GET | `/rankings/new-releases` | auth | Most recently published resources |
| GET | `/recommendations` | auth | Personalized recommendations → `{"data":[{strategy, resources}]}` |

---

## 3. Resources

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/resources` | auth | List resources: `?status=`, `?category_id=`, `?page=`, `?page_size=` |
| POST | `/resources` | auth | Create draft: `title`, `description`, `content_body`, `category_id`, `tags[]` → 303 `/resources/{id}` |
| GET | `/resources/:id` | auth | Resource detail |
| PUT | `/resources/:id` | auth | Update draft/rejected: `title`, `description`, `content_body`, `version` → 303 |
| DELETE | `/resources/:id` | auth | Delete resource |
| GET | `/resources/new` | auth | New resource form |
| GET | `/resources/:id/edit` | auth | Edit form |

### 3a. Resource Workflow

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/resources/:id/submit` | auth | DRAFT → PENDING_REVIEW: `version` → 303 |
| POST | `/resources/:id/approve` | REVIEWER+ | PENDING_REVIEW → APPROVED: `version` → 303 |
| POST | `/resources/:id/reject` | REVIEWER+ | PENDING_REVIEW → REJECTED: `version`, `notes` → 303 |
| POST | `/resources/:id/publish` | ADMIN | APPROVED → PUBLISHED: `version` → 303 |
| POST | `/resources/:id/revise` | auth | REJECTED → DRAFT: `version` → 303 |
| POST | `/resources/:id/takedown` | ADMIN | PUBLISHED → TAKEN_DOWN: `version`, `reason` → 303 |
| POST | `/resources/:id/restore` | ADMIN | TAKEN_DOWN → PUBLISHED: `version` → 303 |

### 3b. Files

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/resources/:id/files` | auth | Upload file (multipart, max 50 MB) |
| GET | `/resources/:id/files/:fileID` | auth | Download file |
| DELETE | `/resources/:id/files/:fileID` | auth | Delete file |

### 3c. Review Queue

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/review-queue` | REVIEWER+ | List PENDING_REVIEW resources |

---

## 4. Tags & Categories

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/tags` | auth | List all tags |
| POST | `/tags` | auth | Create tag: `name` |
| DELETE | `/tags/:id` | ADMIN | Delete tag |
| GET | `/categories` | ADMIN | List categories |
| POST | `/categories` | ADMIN | Create category: `name`, `description`, `parent_id` |
| PUT | `/categories/:id` | ADMIN | Update category |
| DELETE | `/categories/:id` | ADMIN | Delete category |

---

## 5. Engagement

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/resources/:id/vote` | auth | Vote: `direction=up\|down` → 303 |
| DELETE | `/resources/:id/vote` | auth | Retract vote → 303 |
| POST | `/resources/:id/favorite` | auth | Toggle favorite → 303 |
| GET | `/favorites` | auth | List user's favorites |
| POST | `/follows` | auth | Follow author: `target_type=AUTHOR`, `target_id={uuid}` → 303 |

---

## 6. Gamification

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/users/:id/points` | auth | User points + level → `{"total_points":N,"level":N}` |
| GET | `/users/:id/badges` | auth | User badges → `{"badges":[{badge_type,awarded_at}]}` |
| GET | `/leaderboard` | auth | Top users by points |
| GET | `/point-rules` | ADMIN | List point rules |
| PUT | `/point-rules/:id` | ADMIN | Update rule: `points` |

---

## 7. Moderation

### Reports

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/reports` | auth | File report: `resource_id`, `reason`, `details` → 303 |
| GET | `/moderation/reports` | REVIEWER+ | List reports: `?status=`, `?page=` |
| GET | `/moderation/reports/:id` | REVIEWER+ | Report detail |
| POST | `/moderation/reports/:id/assign` | REVIEWER+ | Assign to self |
| POST | `/moderation/reports/:id/resolve` | REVIEWER+ | Resolve report |
| POST | `/moderation/reports/:id/dismiss` | REVIEWER+ | Dismiss report |

### Resource Moderation

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/moderation/resources/:id/takedown` | REVIEWER+ | Takedown: `version`, `reason` |
| POST | `/moderation/resources/:id/restore` | ADMIN | Restore |

### Users

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/moderation/users/:id/ban` | ADMIN | Ban user: `reason`, `duration` (days) |
| POST | `/moderation/users/:id/unban` | ADMIN | Unban user |

### Anomalies

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/moderation/anomalies` | REVIEWER+ | List anomaly flags |
| POST | `/moderation/anomalies/:id/review` | REVIEWER+ | Review flag: `action=dismiss\|escalate` |

---

## 8. Bulk Import / Export

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/import` | ADMIN | Import wizard |
| POST | `/import/upload` | ADMIN | Upload CSV → 303 `/import/{jobID}/preview` |
| GET | `/import/:jobID/preview` | ADMIN | Preview import rows |
| POST | `/import/:jobID/confirm` | ADMIN | Confirm import → creates DRAFT resources |
| GET | `/import/:jobID/done` | ADMIN | Import completion page |
| GET | `/export` | ADMIN | Export page |
| POST | `/export/generate` | ADMIN | Generate CSV export → `{"file_path":"..."}` |

---

## 9. Supplier Portal

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/supplier/portal` | SUPPLIER+ | Supplier dashboard |
| GET | `/supplier/orders` | SUPPLIER+ | List orders |
| GET | `/supplier/orders/new` | SUPPLIER+ | New order form |
| GET | `/supplier/orders/:id` | SUPPLIER+ | Order detail |
| POST | `/supplier/orders/:id/confirm` | SUPPLIER+ | Confirm delivery date: `confirmed_delivery_date` |
| POST | `/supplier/orders/:id/asn` | SUPPLIER+ | Submit ASN: `tracking_number`, `carrier`, `estimated_delivery`, `actual_quantity_sent` |

### Supplier Admin

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/suppliers` | ADMIN | List suppliers |
| POST | `/suppliers` | ADMIN | Create supplier: `name`, `email`, `tier` |
| GET | `/suppliers/:id` | ADMIN | Supplier detail |
| GET | `/suppliers/:id/kpis` | ADMIN | KPI dashboard |
| POST | `/suppliers/:id/kpis/recalculate` | ADMIN | Recalculate KPIs |
| POST | `/supplier/orders` | ADMIN | Create order: `supplier_id`, `item_name`, `quantity`, `unit_price` |
| POST | `/supplier/orders/:id/receive` | ADMIN | Confirm receipt: `received_quantity` |
| POST | `/supplier/orders/:id/qc` | ADMIN | QC result: `passed=true\|false`, `notes` |
| POST | `/supplier/orders/:id/close` | ADMIN | Close order |
| POST | `/supplier/orders/:id/cancel` | ADMIN | Cancel order |

---

## 10. Messaging

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/messaging` | auth | Messaging center (full page) |
| GET | `/messaging/notifications` | auth | Notification list: `?is_read=all\|read\|unread`, `?event_type=`, `?page=` → `{"notifications":[...],"unread_count":N}` |
| POST | `/messaging/notifications/:id/read` | auth | Mark single notification read → `{"ok":true}` |
| POST | `/messaging/notifications/read-all` | auth | Mark all notifications read → `{"ok":true}` |
| GET | `/messaging/subscriptions` | auth | List subscriptions → `{"subscriptions":[{event_type,enabled}]}` |
| PUT | `/messaging/subscriptions` | auth | Update subscription: `event_type`, `enabled=true\|false` → `{"ok":true}` |
| GET | `/events/stream` | auth | SSE stream — emits `event: notification` with JSON payload |

---

## 11. Analytics

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/analytics/dashboard` | auth | Dashboard metrics (role-filtered) → `{"metrics":{...}}` |
| GET | `/analytics/reports` | ADMIN | List generated reports |
| POST | `/analytics/reports/generate` | ADMIN | Generate report: `report_type=ANALYTICS` → `{"report_id":"..."}` |
| GET | `/analytics/reports/:id/download` | ADMIN | Download report CSV file |
| GET | `/audit-logs` | ADMIN | Audit log: `?actor_id=`, `?entity_type=`, `?action=`, `?from=`, `?to=`, `?page=` → `{"entries":[...],"total":N}` |
| POST | `/audit-logs/export` | ADMIN | Export audit log CSV → `{"file_path":"..."}` |

---

## 12. Admin — User Management

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/admin/users` | ADMIN | List users: `?page=`, `?search=` |
| GET | `/admin/users/:id` | ADMIN | User detail |
| POST | `/admin/users/:id/status` | ADMIN | Change status: `status=ACTIVE\|SUSPENDED` |
| POST | `/admin/users/:id/roles/assign` | ADMIN | Assign role: `role=ADMIN\|AUTHOR\|REVIEWER\|SUPPLIER` |
| POST | `/admin/users/:id/roles/remove` | ADMIN | Remove role |
| POST | `/admin/users/:id/unlock` | ADMIN | Unlock locked account |

### Recommendation Strategies

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/recommendation-strategies` | ADMIN | List strategy configs |
| PUT | `/recommendation-strategies/:id` | ADMIN | Update strategy: `weight`, `enabled=true\|false` |

---

## Error Responses

| Status | Meaning |
|--------|---------|
| 303 | Success mutation — follow Location header |
| 400 | Bad request / validation error |
| 401 | Not authenticated (no cookie) |
| 403 | Authenticated but insufficient role |
| 404 | Resource not found |
| 409 | Optimistic lock conflict (stale version) |
| 422 | Unprocessable entity / invalid state transition |
| 429 | Rate limit exceeded (20 resource creates/hour) |
| 500 | Internal server error |

---

## Real-Time (SSE)

Connect to `GET /events/stream` with session cookie. The server pushes events as:

```
event: notification
data: {"id":"...","event_type":"badge_earned","title":"...","body":"..."}

event: ping
data: connected
```

The frontend JS updates the bell badge count and inserts toast notifications.

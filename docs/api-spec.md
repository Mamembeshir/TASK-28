# EduExchange API Specification

## Overview

This document provides a complete reference for all API endpoints in the EduExchange platform. All authenticated endpoints require a valid `session_token` cookie.

---

## Authentication Endpoints

Public endpoints for user registration and login.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/login` | Public | Display login form |
| POST | `/login` | Public | Authenticate user with username/password |
| GET | `/register` | Public | Display registration form |
| POST | `/register` | Public | Create new user account |

### Login (POST `/login`)
- **Request**: Form fields: `username`, `password`
- **Response**: 200/302 on success with `session_token` cookie; 422 on validation error
- **Notes**: Sets `session_token` cookie on successful authentication

### Register (POST `/register`)
- **Request**: Form fields: `username`, `email`, `password`
- **Response**: 200/302 on success; 422 on validation error (duplicate username, weak password, invalid email)
- **Validations**: Password must meet security requirements, email must be valid, username must be unique

### Logout (POST `/logout`)
- **Auth**: Authenticated
- **Request**: None
- **Response**: 302 redirect

---

## Resource Endpoints

Manage educational resources through DRAFT → PENDING_REVIEW → APPROVED → PUBLISHED workflow.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/resources` | Authenticated | List resources accessible to user |
| GET | `/resources/new` | Authenticated | Display resource creation form |
| POST | `/resources` | Authenticated | Create new resource (rate-limited to 20/period) |
| GET | `/resources/:id` | Authenticated | View resource details |
| GET | `/resources/:id/edit` | Authenticated | Display edit form for resource |
| PUT | `/resources/:id` | Authenticated | Update resource (DRAFT or REJECTED only) |
| DELETE | `/resources/:id` | Authenticated | Delete resource (DRAFT only) |

### Create Resource (POST `/resources`)
- **Request**: Form fields: `title`, `description`, `content_body`
- **Response**: 303 redirect to `/resources/{id}`
- **Status**: New resources begin in DRAFT state

### Get Resource Detail (GET `/resources/:id`)
- **Response**: 200 with HTML/JSON containing: `id`, `title`, `description`, `status`, `created_at`

---

## Resource Workflow Endpoints

Control resource review and publication workflow.

| Method | Path | Auth | Role | Description |
|--------|------|------|------|-------------|
| POST | `/resources/:id/submit` | Authenticated | Author | Submit for review (DRAFT → PENDING_REVIEW) |
| POST | `/resources/:id/revise` | Authenticated | Author | Return to draft (REJECTED → DRAFT) |
| POST | `/resources/:id/approve` | Authenticated | REVIEWER+ | Approve resource (PENDING_REVIEW → APPROVED) |
| POST | `/resources/:id/reject` | Authenticated | REVIEWER+ | Reject resource (PENDING_REVIEW → REJECTED) |
| POST | `/resources/:id/publish` | Authenticated | ADMIN | Publish resource (APPROVED → PUBLISHED) |
| POST | `/resources/:id/takedown` | Authenticated | ADMIN | Remove from publication (PUBLISHED → TAKEN_DOWN) |
| POST | `/resources/:id/restore` | Authenticated | ADMIN | Restore taken-down resource |

### Submit for Review (POST `/resources/:id/submit`)
- **Request**: Form fields: `version` (optimistic locking)
- **Response**: 303 redirect
- **Notes**: Author must own the resource; transitions DRAFT → PENDING_REVIEW

### Approve (POST `/resources/:id/approve`)
- **Request**: Form fields: `version`
- **Response**: 303 redirect
- **Role**: REVIEWER or ADMIN

### Reject (POST `/resources/:id/reject`)
- **Request**: Form fields: `version`, `notes` (optional rejection reason)
- **Response**: 303 redirect

### Publish (POST `/resources/:id/publish`)
- **Request**: Form fields: `version`
- **Response**: 303 redirect
- **Role**: ADMIN only

---

## File Management Endpoints

Upload and manage files attached to resources.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/resources/:id/files` | Authenticated | Upload file to resource |
| GET | `/resources/:id/files/:fileID` | Authenticated | Download file |
| DELETE | `/resources/:id/files/:fileID` | Authenticated | Delete file from resource |

### Upload File (POST `/resources/:id/files`)
- **Request**: Multipart form with file
- **Response**: 303 redirect

### Download File (GET `/resources/:id/files/:fileID`)
- **Response**: 200 with binary file content

---

## Tag Management Endpoints

| Method | Path | Auth | Role | Description |
|--------|------|------|------|-------------|
| GET | `/tags` | Authenticated | - | List all tags |
| POST | `/tags` | Authenticated | - | Create new tag |
| DELETE | `/tags/:id` | Authenticated | ADMIN | Delete tag |

### Get Tags (GET `/tags`)
- **Response**: 200 with JSON: `tags` array containing `id`, `name`, `created_at`

### Create Tag (POST `/tags`)
- **Request**: Form fields: `name`
- **Response**: 303 redirect

---

## Category Management Endpoints

Admin-only endpoints for managing resource categories.

| Method | Path | Auth | Role | Description |
|--------|------|------|------|-------------|
| GET | `/categories` | Authenticated | ADMIN | List all categories |
| POST | `/categories` | Authenticated | ADMIN | Create category |
| PUT | `/categories/:id` | Authenticated | ADMIN | Update category |
| DELETE | `/categories/:id` | Authenticated | ADMIN | Delete category |

### Get Categories (GET `/categories`)
- **Response**: 200 with JSON: `categories` array with `id`, `name`, `slug`

---

## Review Queue Endpoints

Reviewer/Admin workflow for resource review.

| Method | Path | Auth | Role | Description |
|--------|------|------|------|-------------|
| GET | `/review-queue` | Authenticated | REVIEWER+ | View resources pending review |

### Get Review Queue (GET `/review-queue`)
- **Response**: 200 with HTML/JSON: list of PENDING_REVIEW resources with `id`, `title`, `author`, `submitted_at`

---

## Engagement Endpoints

User interactions: voting, favorites, follows.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| POST | `/resources/:id/vote` | Authenticated | Cast or change vote (UP/DOWN) |
| DELETE | `/resources/:id/vote` | Authenticated | Remove vote |
| POST | `/resources/:id/favorite` | Authenticated | Add/remove favorite |
| GET | `/favorites` | Authenticated | List user's favorite resources |
| POST | `/follows` | Authenticated | Follow/unfollow user |

### Cast Vote (POST `/resources/:id/vote`)
- **Request**: JSON: `{ "vote_type": "UP" | "DOWN" }`
- **Response**: 200 with JSON: `{ "upvotes": number, "downvotes": number }`
- **Notes**: Cannot vote on own resources; replaces previous vote

### Delete Vote (DELETE `/resources/:id/vote`)
- **Response**: 200

### Add Favorite (POST `/resources/:id/favorite`)
- **Request**: Form fields: none
- **Response**: 303 redirect

### Get Favorites (GET `/favorites`)
- **Response**: 200 with HTML/JSON: list of favorited resources

### Follow User (POST `/follows`)
- **Request**: Form fields: `user_id`
- **Response**: 303 redirect

---

## Gamification Endpoints

User points, badges, and leaderboard system.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/users/:id/points` | Authenticated | Get user's point totals and level |
| GET | `/users/:id/badges` | Authenticated | Get user's earned badges |
| GET | `/leaderboard` | Authenticated | Get top users by points |

### Get User Points (GET `/users/:id/points`)
- **Response**: 200 with JSON: `{ "points": { "total_points": number, "level": number, "breakdown": { ... } } }`

### Get User Badges (GET `/users/:id/badges`)
- **Response**: 200 with JSON: `{ "badges": array }`
- **Fields**: `id`, `name`, `description`, `earned_at`

### Get Leaderboard (GET `/leaderboard`)
- **Response**: 200 with JSON: `{ "data": array of users with `{ "rank": number, "username": string, "total_points": number } }`

### Admin: Point Rules (GET/PUT)

| Method | Path | Auth | Role | Description |
|--------|------|------|------|-------------|
| GET | `/point-rules` | Authenticated | ADMIN | List point rules |
| PUT | `/point-rules/:id` | Authenticated | ADMIN | Update point rule value |

---

## Search Endpoints

Find resources and get recommendations.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/search` | Authenticated | Full-text search resources (query param: `q`) |
| GET | `/search/suggest` | Authenticated | Get search suggestions |
| GET | `/search/history` | Authenticated | Get user's search history |
| DELETE | `/search/history` | Authenticated | Clear search history |
| GET | `/rankings/bestsellers` | Authenticated | Get top-rated resources |
| GET | `/rankings/new-releases` | Authenticated | Get newest resources |
| GET | `/recommendations` | Authenticated | Get personalized recommendations |

### Search (GET `/search?q=query`)
- **Query Params**: `q` (search term)
- **Response**: 200 with JSON: array of matching resources with `id`, `title`, `description`

### Search Suggestions (GET `/search/suggest?q=query`)
- **Response**: 200 with JSON: array of suggestion strings

### Leaderboard Recommendations (GET `/recommendation-strategies`)
- **Auth**: ADMIN
- **Response**: 200 with JSON: list of strategy configs with `id`, `name`, `weight`

### Update Recommendation Strategy (PUT `/recommendation-strategies/:id`)
- **Auth**: ADMIN
- **Request**: Form fields: strategy configuration
- **Response**: 303 redirect

---

## Report & Moderation Endpoints

User-submitted reports and moderation queue.

| Method | Path | Auth | Role | Description |
|--------|------|------|------|-------------|
| POST | `/reports` | Authenticated | - | Create abuse/spam report |

### Create Report (POST `/reports`)
- **Request**: Form fields: `resource_id`, `reason_type` (SPAM, ABUSE, COPYRIGHT, etc.), `description`
- **Response**: 303 redirect
- **Notes**: Any authenticated user can report

---

## Moderation Queue Endpoints

Reviewer/Admin moderation workflow.

| Method | Path | Auth | Role | Description |
|--------|------|------|------|-------------|
| GET | `/moderation/reports` | Authenticated | REVIEWER+ | List reports |
| GET | `/moderation/reports/:id` | Authenticated | REVIEWER+ | View report details |
| POST | `/moderation/reports/:id/assign` | Authenticated | REVIEWER+ | Assign report to self |
| POST | `/moderation/reports/:id/resolve` | Authenticated | REVIEWER+ | Resolve report (take action) |
| POST | `/moderation/reports/:id/dismiss` | Authenticated | REVIEWER+ | Dismiss unfounded report |
| POST | `/moderation/resources/:id/takedown` | Authenticated | REVIEWER+ | Remove resource from platform |
| GET | `/moderation/anomalies` | Authenticated | REVIEWER+ | List anomaly flags |
| POST | `/moderation/anomalies/:id/review` | Authenticated | REVIEWER+ | Review anomaly flag |

### List Reports (GET `/moderation/reports`)
- **Response**: 200 with JSON: array of reports with `id`, `resource_id`, `reason_type`, `status`, `reported_at`

### Resolve Report (POST `/moderation/reports/:id/resolve`)
- **Request**: Form fields: `action` (TAKEDOWN, WARNING, etc.)
- **Response**: 303 redirect

---

## Admin Moderation Endpoints

Admin-only moderation actions.

| Method | Path | Auth | Role | Description |
|--------|------|------|------|-------------|
| POST | `/moderation/resources/:id/restore` | Authenticated | ADMIN | Restore taken-down resource |
| POST | `/moderation/users/:id/ban` | Authenticated | ADMIN | Ban user account |
| POST | `/moderation/users/:id/unban` | Authenticated | ADMIN | Unban user account |

### Ban User (POST `/moderation/users/:id/ban`)
- **Request**: Form fields: `reason`
- **Response**: 303 redirect or 200 JSON

### Unban User (POST `/moderation/users/:id/unban`)
- **Response**: 303 redirect or 200 JSON

---

## Import/Export Endpoints

Bulk resource management (Admin only).

| Method | Path | Auth | Role | Description |
|--------|------|------|------|-------------|
| GET | `/import` | Authenticated | ADMIN | Display import wizard |
| POST | `/import/upload` | Authenticated | ADMIN | Upload CSV/Excel file |
| GET | `/import/:jobID/preview` | Authenticated | ADMIN | Preview imported data |
| POST | `/import/:jobID/confirm` | Authenticated | ADMIN | Confirm and commit import |
| GET | `/import/:jobID/done` | Authenticated | ADMIN | Import completion page |
| GET | `/export` | Authenticated | ADMIN | Display export page |
| POST | `/export/generate` | Authenticated | ADMIN | Generate export file |

### Upload Import (POST `/import/upload`)
- **Request**: Multipart form with file
- **Response**: 303 redirect with jobID

### Confirm Import (POST `/import/:jobID/confirm`)
- **Response**: 303 redirect
- **Notes**: Processes and commits imported resources

---

## Supplier Portal Endpoints

Supplier management and order workflow.

| Method | Path | Auth | Role | Description |
|--------|------|------|------|-------------|
| GET | `/supplier/portal` | Authenticated | SUPPLIER+ | Supplier dashboard |
| GET | `/supplier/orders` | Authenticated | SUPPLIER+ | List supplier orders |
| GET | `/supplier/orders/new` | Authenticated | SUPPLIER+ | New order form |
| GET | `/supplier/orders/:id` | Authenticated | SUPPLIER+ | Order details |
| PUT | `/supplier/orders/:id/confirm` | Authenticated | SUPPLIER+ | Confirm delivery date |
| POST | `/supplier/orders/:id/confirm` | Authenticated | SUPPLIER+ | Confirm delivery date |
| POST | `/supplier/orders/:id/asn` | Authenticated | SUPPLIER+ | Submit ASN (Advanced Ship Notice) |

### Confirm Delivery (PUT/POST `/supplier/orders/:id/confirm`)
- **Request**: Form fields: `delivery_date`
- **Response**: 303 redirect or 200 JSON

### Submit ASN (POST `/supplier/orders/:id/asn`)
- **Request**: Form fields: ASN data (tracking, items, etc.)
- **Response**: 303 redirect or 200 JSON

---

## Admin Supplier Management

Admin order and supplier management.

| Method | Path | Auth | Role | Description |
|--------|------|------|------|-------------|
| GET | `/suppliers` | Authenticated | ADMIN | List all suppliers |
| POST | `/suppliers` | Authenticated | ADMIN | Create supplier |
| GET | `/suppliers/:id` | Authenticated | ADMIN | Supplier details |
| GET | `/suppliers/:id/kpis` | Authenticated | ADMIN | KPI dashboard |
| POST | `/suppliers/:id/kpis/recalculate` | Authenticated | ADMIN | Recalculate KPI metrics |
| POST | `/supplier/orders` | Authenticated | ADMIN | Create purchase order |
| POST | `/supplier/orders/:id/receive` | Authenticated | ADMIN | Confirm receipt |
| POST | `/supplier/orders/:id/qc` | Authenticated | ADMIN | Submit QC result |
| POST | `/supplier/orders/:id/close` | Authenticated | ADMIN | Close order |
| POST | `/supplier/orders/:id/cancel` | Authenticated | ADMIN | Cancel order |

### Create Supplier (POST `/suppliers`)
- **Request**: Form fields: `name`, `email`, `contact_info`
- **Response**: 303 redirect or 200 JSON

### Create Order (POST `/supplier/orders`)
- **Request**: Form fields: `supplier_id`, `sku`, `description`, `quantity`, `unit_price`
- **Response**: 201 with JSON: `{ "id": uuid, ... }`

### Confirm Receipt (POST `/supplier/orders/:id/receive`)
- **Response**: 303 redirect or 200 JSON

### Submit QC (POST `/supplier/orders/:id/qc`)
- **Request**: Form fields: `qc_status` (PASS, FAIL), `notes`
- **Response**: 303 redirect or 200 JSON

### Close Order (POST `/supplier/orders/:id/close`)
- **Response**: 303 redirect or 200 JSON

---

## Messaging Endpoints

Notifications and messaging center.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/messaging` | Authenticated | Messaging center page |
| GET | `/messaging/notifications` | Authenticated | List user notifications (JSON) |
| POST | `/messaging/notifications/:id/read` | Authenticated | Mark notification as read |
| POST | `/messaging/notifications/read-all` | Authenticated | Mark all notifications as read |
| GET | `/messaging/subscriptions` | Authenticated | Get notification subscriptions |
| PUT | `/messaging/subscriptions` | Authenticated | Update subscriptions |
| GET | `/events/stream` | Authenticated | Server-sent events stream |

### Get Notifications (GET `/messaging/notifications`)
- **Response**: 200 with JSON: `{ "notifications": array of { "id", "event_type", "title", "body", "is_read", "created_at" } }`

### Mark Read (POST `/messaging/notifications/:id/read`)
- **Response**: 200

### Mark All Read (POST `/messaging/notifications/read-all`)
- **Response**: 200

### Get Subscriptions (GET `/messaging/subscriptions`)
- **Response**: 200 with JSON: `{ "subscriptions": { "badge_earned": boolean, "level_up": boolean, ... } }`

### Update Subscriptions (PUT `/messaging/subscriptions`)
- **Request**: JSON: subscription preferences
- **Response**: 200

### Event Stream (GET `/events/stream`)
- **Response**: 200 with Server-Sent Events (Content-Type: text/event-stream)
- **Notes**: Persistent connection for real-time updates

---

## Analytics Endpoints

Analytics dashboard and reporting.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/analytics/dashboard` | Authenticated | Analytics dashboard (JSON) |
| GET | `/analytics/reports` | Authenticated | List reports (ADMIN) |
| POST | `/analytics/reports/generate` | Authenticated | Generate report (ADMIN) |
| GET | `/analytics/reports/:id/download` | Authenticated | Download report file (ADMIN) |
| GET | `/audit-logs` | Authenticated | Audit log entries (ADMIN) |
| POST | `/audit-logs/export` | Authenticated | Export audit logs (ADMIN) |

### Get Dashboard (GET `/analytics/dashboard`)
- **Response**: 200 with JSON: `{ "metrics": { ... } }`
- **Fields**: resource count, total votes, user counts, etc.

### Generate Report (POST `/analytics/reports/generate`)
- **Request**: Form fields: `report_type` (ANALYTICS, USAGE, etc.)
- **Response**: 200 with JSON: `{ "report_id": uuid }`
- **Role**: ADMIN

### Get Report Download (GET `/analytics/reports/:id/download`)
- **Response**: 200 with file (PDF/CSV)
- **Role**: ADMIN

### Get Audit Logs (GET `/audit-logs`)
- **Response**: 200 with JSON: array of audit entries
- **Role**: ADMIN
- **Fields**: `id`, `user_id`, `action`, `resource_type`, `resource_id`, `timestamp`

---

## Admin User Management

Administrative user and role management.

| Method | Path | Auth | Role | Description |
|--------|------|------|------|-------------|
| GET | `/admin/users` | Authenticated | ADMIN | List all users |
| GET | `/admin/users/:id` | Authenticated | ADMIN | User details |
| POST | `/admin/users/:id/status` | Authenticated | ADMIN | Transition user status |
| POST | `/admin/users/:id/roles/assign` | Authenticated | ADMIN | Assign role to user |
| POST | `/admin/users/:id/roles/remove` | Authenticated | ADMIN | Remove role from user |
| POST | `/admin/users/:id/unlock` | Authenticated | ADMIN | Unlock locked user account |

### List Users (GET `/admin/users`)
- **Response**: 200 with HTML/JSON: array of users with `id`, `username`, `email`, `roles`, `status`

### Get User Detail (GET `/admin/users/:id`)
- **Response**: 200 with user info including `roles`, `status`, `created_at`, `last_login`

### Transition Status (POST `/admin/users/:id/status`)
- **Request**: Form fields: `status` (ACTIVE, SUSPENDED, DEACTIVATED), `version` (optimistic locking)
- **Response**: 200 on success; 422 on invalid transition; 409 on stale version
- **Valid Transitions**:
  - ACTIVE ↔ SUSPENDED (reversible)
  - ACTIVE → DEACTIVATED (terminal)
  - Locked → ACTIVE (unlock via separate endpoint)

### Assign Role (POST `/admin/users/:id/roles/assign`)
- **Request**: Form fields: `role` (REGULAR_USER, AUTHOR, REVIEWER, SUPPLIER, ADMIN)
- **Response**: 200 or 303 redirect

### Remove Role (POST `/admin/users/:id/roles/remove`)
- **Request**: Form fields: `role`
- **Response**: 422 if removing last role (users must have at least one)

### Unlock User (POST `/admin/users/:id/unlock`)
- **Response**: 200 or 303 redirect
- **Notes**: Resets failed login attempts

---

## Status Codes

| Code | Meaning |
|------|---------|
| 200 | Success (GET, POST, PUT) |
| 201 | Created (POST creating resource returns object) |
| 302 | Redirect (form submission success) |
| 303 | See Other (form submission with redirect) |
| 400 | Bad Request (malformed input) |
| 401 | Unauthorized (no/invalid session token) |
| 403 | Forbidden (insufficient role/permission) |
| 404 | Not Found (resource doesn't exist) |
| 409 | Conflict (optimistic lock version mismatch) |
| 422 | Unprocessable Entity (validation error) |
| 500 | Internal Server Error |

---

## Common Query Parameters

- `q` – Search query string (for `/search` endpoints)
- `version` – Optimistic locking version number (for state-changing form submissions)
- `role` – User role filter (for admin endpoints)
- `status` – Filter by status (e.g., PUBLISHED, DRAFT, PENDING_REVIEW)

---

## Authentication

All protected endpoints require a valid `session_token` cookie set via `/login`. The session is automatically established after successful authentication and persisted in the user's browser. Unauthenticated requests to protected endpoints receive a 401 Unauthorized response.

---

## Optimistic Locking

State-changing operations (status transitions, resource updates) use a `version` field for optimistic concurrency control. The client must:
1. Fetch the current version from the resource
2. Include it in the request form/body
3. Handle 409 Conflict responses by re-fetching and retrying

---

## Error Handling

Validation and business logic errors return 422 Unprocessable Entity with error details in HTML or JSON. Form-based endpoints typically re-render with error messages. JSON endpoints return structured error responses.


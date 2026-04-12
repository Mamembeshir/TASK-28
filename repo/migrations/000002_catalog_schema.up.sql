-- ============================================
-- Catalog: Categories (hierarchical, max 3 levels)
-- ============================================

CREATE TABLE categories (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(200) NOT NULL,
    parent_id UUID REFERENCES categories(id) ON DELETE SET NULL,
    level INT NOT NULL DEFAULT 1 CHECK (level BETWEEN 1 AND 3),
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_categories_parent_id ON categories(parent_id);
CREATE INDEX idx_categories_level ON categories(level);

-- ============================================
-- Catalog: Tags
-- ============================================

CREATE TABLE tags (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(100) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tags_name ON tags(name);

-- ============================================
-- Catalog: Resources
-- ============================================

CREATE TABLE resources (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    title VARCHAR(300) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    content_body TEXT NOT NULL DEFAULT '',
    author_id UUID NOT NULL REFERENCES users(id),
    category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT','PENDING_REVIEW','APPROVED','PUBLISHED','REJECTED','TAKEN_DOWN')),
    current_version_number INT NOT NULL DEFAULT 1,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_resources_author_id ON resources(author_id);
CREATE INDEX idx_resources_category_id ON resources(category_id);
CREATE INDEX idx_resources_status ON resources(status);
CREATE INDEX idx_resources_created_at ON resources(created_at DESC);
CREATE INDEX idx_resources_title_trgm ON resources USING gin(title gin_trgm_ops);

-- ============================================
-- Catalog: Resource Versions (immutable snapshots)
-- ============================================

CREATE TABLE resource_versions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    resource_id UUID NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    version_number INT NOT NULL,
    data_snapshot JSONB NOT NULL,
    changed_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (resource_id, version_number)
);

CREATE INDEX idx_resource_versions_resource_id ON resource_versions(resource_id);

-- ============================================
-- Catalog: Resource Tags (M2M)
-- ============================================

CREATE TABLE resource_tags (
    resource_id UUID NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (resource_id, tag_id)
);

CREATE INDEX idx_resource_tags_tag_id ON resource_tags(tag_id);

-- ============================================
-- Catalog: Resource Files
-- ============================================

CREATE TABLE resource_files (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    resource_id UUID NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    original_name VARCHAR(500) NOT NULL,
    stored_path TEXT NOT NULL,
    mime_type VARCHAR(200) NOT NULL,
    size_bytes BIGINT NOT NULL,
    sha256 CHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_resource_files_resource_id ON resource_files(resource_id);

-- ============================================
-- Catalog: Bulk Import Jobs
-- ============================================

CREATE TABLE bulk_import_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    uploaded_by UUID NOT NULL REFERENCES users(id),
    file_path TEXT NOT NULL,
    original_filename VARCHAR(500) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING','PROCESSING','PREVIEW_READY','CONFIRMED','FAILED')),
    total_rows INT NOT NULL DEFAULT 0,
    valid_rows INT NOT NULL DEFAULT 0,
    invalid_rows INT NOT NULL DEFAULT 0,
    results_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_bulk_import_jobs_uploaded_by ON bulk_import_jobs(uploaded_by);
CREATE INDEX idx_bulk_import_jobs_status ON bulk_import_jobs(status);

-- ============================================
-- Catalog: Review actions (approve/reject notes)
-- ============================================

CREATE TABLE resource_reviews (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    resource_id UUID NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    reviewer_id UUID NOT NULL REFERENCES users(id),
    action VARCHAR(20) NOT NULL CHECK (action IN ('APPROVED','REJECTED')),
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_resource_reviews_resource_id ON resource_reviews(resource_id);

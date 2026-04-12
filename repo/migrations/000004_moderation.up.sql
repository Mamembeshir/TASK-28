CREATE TABLE reports (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    reporter_id UUID NOT NULL REFERENCES users(id),
    resource_id UUID NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    reason_type VARCHAR(20) NOT NULL CHECK (reason_type IN ('SPAM','INAPPROPRIATE','COPYRIGHT','OTHER')),
    description TEXT NOT NULL DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','UNDER_REVIEW','RESOLVED','DISMISSED')),
    reviewer_id UUID REFERENCES users(id),
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_reports_resource_id ON reports(resource_id);
CREATE INDEX idx_reports_status ON reports(status);
CREATE INDEX idx_reports_reporter_id ON reports(reporter_id);

CREATE TABLE moderation_actions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    moderator_id UUID NOT NULL REFERENCES users(id),
    action_type VARCHAR(20) NOT NULL CHECK (action_type IN ('APPROVE','REJECT','TAKEDOWN','RESTORE','WARN','BAN')),
    target_type VARCHAR(20) NOT NULL CHECK (target_type IN ('RESOURCE','USER')),
    target_id UUID NOT NULL,
    report_id UUID REFERENCES reports(id),
    notes TEXT NOT NULL DEFAULT '',
    evidence_json JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_moderation_actions_target_id ON moderation_actions(target_id);
CREATE INDEX idx_moderation_actions_moderator_id ON moderation_actions(moderator_id);

CREATE TABLE user_bans (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    ban_type VARCHAR(10) NOT NULL CHECK (ban_type IN ('1_DAY','7_DAYS','PERMANENT')),
    reason TEXT NOT NULL DEFAULT '',
    banned_by UUID NOT NULL REFERENCES users(id),
    expires_at TIMESTAMPTZ,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_user_bans_user_id ON user_bans(user_id);
CREATE INDEX idx_user_bans_is_active ON user_bans(is_active);

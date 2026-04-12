-- ============================================
-- Engagement: Votes
-- ============================================

CREATE TABLE votes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    resource_id UUID NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    vote_type VARCHAR(4) NOT NULL CHECK (vote_type IN ('UP', 'DOWN')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, resource_id)
);

CREATE INDEX idx_votes_resource_id ON votes(resource_id);
CREATE INDEX idx_votes_user_id ON votes(user_id);

-- ============================================
-- Engagement: Favorites
-- ============================================

CREATE TABLE favorites (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    resource_id UUID NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, resource_id)
);

CREATE INDEX idx_favorites_user_id ON favorites(user_id);
CREATE INDEX idx_favorites_resource_id ON favorites(resource_id);

-- ============================================
-- Engagement: Follows
-- ============================================

CREATE TABLE follows (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    follower_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_type VARCHAR(10) NOT NULL CHECK (target_type IN ('AUTHOR', 'TOPIC')),
    target_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (follower_id, target_type, target_id)
);

CREATE INDEX idx_follows_follower_id ON follows(follower_id);
CREATE INDEX idx_follows_target ON follows(target_type, target_id);

-- ============================================
-- Gamification: User Points
-- ============================================

CREATE TABLE user_points (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    total_points INT NOT NULL DEFAULT 0,
    level INT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================
-- Gamification: Point Transactions
-- ============================================

CREATE TABLE point_transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    points INT NOT NULL,
    reason VARCHAR(200) NOT NULL,
    source_type VARCHAR(50) NOT NULL,
    source_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_point_transactions_user_id ON point_transactions(user_id);
CREATE INDEX idx_point_transactions_source ON point_transactions(source_type, source_id);

-- ============================================
-- Gamification: Point Rules (configurable)
-- ============================================

CREATE TABLE point_rules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_type VARCHAR(50) UNIQUE NOT NULL,
    points INT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO point_rules (id, event_type, points, description) VALUES
    (uuid_generate_v4(), 'ENTRY_APPROVED',    5,   'Resource entry approved'),
    (uuid_generate_v4(), 'UPVOTE_RECEIVED',   1,   'Received an upvote'),
    (uuid_generate_v4(), 'DOWNVOTE_RECEIVED', -1,  'Received a downvote'),
    (uuid_generate_v4(), 'FAVORITE_RECEIVED', 2,   'Resource added to favorites'),
    (uuid_generate_v4(), 'TAKEDOWN_PENALTY',  -10, 'Resource taken down by moderator');

-- ============================================
-- Gamification: Badges
-- ============================================

CREATE TABLE badges (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    threshold_type VARCHAR(30) NOT NULL,
    threshold_value INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO badges (id, code, name, description, threshold_type, threshold_value) VALUES
    (uuid_generate_v4(), 'POPULAR_50',  'Community Favorite', 'Received 50 favorites',     'FAVORITES_RECEIVED', 50),
    (uuid_generate_v4(), 'AUTHOR_10',   'Prolific Author',     'Had 10 resources approved', 'ENTRIES_APPROVED',   10),
    (uuid_generate_v4(), 'UPVOTES_100', 'Highly Rated',        'Received 100 upvotes',      'UPVOTES_RECEIVED',   100),
    (uuid_generate_v4(), 'POINT_500',   'Rising Star',         'Earned 500 total points',   'TOTAL_POINTS',       500),
    (uuid_generate_v4(), 'POINT_1000',  'Expert Contributor',  'Earned 1000 total points',  'TOTAL_POINTS',       1000);

-- ============================================
-- Gamification: User Badges
-- ============================================

CREATE TABLE user_badges (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    badge_id UUID NOT NULL REFERENCES badges(id) ON DELETE CASCADE,
    awarded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, badge_id)
);

CREATE INDEX idx_user_badges_user_id ON user_badges(user_id);

-- ============================================
-- Gamification: Ranking Archives
-- ============================================

CREATE TABLE ranking_archives (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    week_number INT NOT NULL,
    year INT NOT NULL,
    ranking_type VARCHAR(15) NOT NULL CHECK (ranking_type IN ('BESTSELLER', 'NEW_RELEASE')),
    entries_json JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (week_number, year, ranking_type)
);

CREATE INDEX idx_ranking_archives_week ON ranking_archives(year, week_number);

-- ============================================
-- Gamification: Recommendation Strategy Config
-- ============================================

CREATE TABLE recommendation_strategy_config (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    strategy_key VARCHAR(100) UNIQUE NOT NULL,
    label VARCHAR(200) NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO recommendation_strategy_config (id, strategy_key, label, sort_order) VALUES
    (uuid_generate_v4(), 'MostEngagedCategories',   'Popular in Your Categories',  1),
    (uuid_generate_v4(), 'FollowedAuthorNewContent', 'From Authors You Follow',     2),
    (uuid_generate_v4(), 'SimilarTagAffinity',       'Based on Your Interests',     3);

-- ============================================
-- Search: Pinyin Mapping
-- ============================================

CREATE TABLE pinyin_mapping (
    character VARCHAR(10) PRIMARY KEY,
    pinyin VARCHAR(100) NOT NULL
);

-- Common Chinese characters → pinyin (education domain)
INSERT INTO pinyin_mapping (character, pinyin) VALUES
    ('教', 'jiao'), ('学', 'xue'), ('数', 'shu'), ('语', 'yu'),
    ('文', 'wen'), ('历', 'li'), ('史', 'shi'), ('地', 'di'),
    ('理', 'li'), ('化', 'hua'), ('物', 'wu'), ('生', 'sheng'),
    ('英', 'ying'), ('音', 'yin'), ('乐', 'le'), ('美', 'mei'),
    ('术', 'shu'), ('体', 'ti'), ('育', 'yu'), ('计', 'ji'),
    ('算', 'suan'), ('机', 'ji'), ('科', 'ke'), ('技', 'ji'),
    ('政', 'zheng'), ('治', 'zhi'), ('经', 'jing'), ('济', 'ji'),
    ('社', 'she'), ('会', 'hui'), ('工', 'gong'), ('程', 'cheng'),
    ('言', 'yan'), ('国', 'guo'), ('际', 'ji'), ('人', 'ren'),
    ('民', 'min'), ('中', 'zhong'), ('大', 'da'), ('小', 'xiao'),
    ('高', 'gao'), ('低', 'di'), ('年', 'nian'), ('级', 'ji'),
    ('班', 'ban'), ('课', 'ke'), ('本', 'ben'), ('书', 'shu'),
    ('读', 'du'), ('写', 'xie'), ('说', 'shuo'), ('听', 'ting'),
    ('看', 'kan'), ('做', 'zuo'), ('用', 'yong'), ('行', 'xing'),
    ('时', 'shi'), ('间', 'jian'), ('日', 'ri'), ('月', 'yue'),
    ('周', 'zhou'), ('期', 'qi'), ('考', 'kao'), ('试', 'shi'),
    ('题', 'ti'), ('答', 'da'), ('问', 'wen'), ('解', 'jie'),
    ('法', 'fa'), ('思', 'si'), ('想', 'xiang'), ('方', 'fang'),
    ('式', 'shi'), ('知', 'zhi'), ('识', 'shi'), ('能', 'neng'),
    ('力', 'li'), ('情', 'qing'), ('感', 'gan'), ('心', 'xin'),
    ('明', 'ming'), ('智', 'zhi'), ('德', 'de'), ('哲', 'zhe'),
    ('发', 'fa'), ('展', 'zhan'), ('创', 'chuang'), ('新', 'xin'),
    ('研', 'yan'), ('究', 'jiu'), ('实', 'shi'), ('践', 'jian'),
    ('应', 'ying'), ('变', 'bian'), ('基', 'ji'), ('础', 'chu'),
    ('进', 'jin'), ('步', 'bu'), ('提', 'ti'), ('难', 'nan'),
    ('复', 'fu'), ('习', 'xi'), ('预', 'yu'), ('备', 'bei'),
    ('多', 'duo'), ('少', 'shao'), ('好', 'hao'), ('差', 'cha');

-- ============================================
-- Search: Synonym Groups
-- ============================================

CREATE TABLE synonym_groups (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    canonical_term VARCHAR(200) NOT NULL,
    synonyms TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_synonym_groups_canonical ON synonym_groups(canonical_term);

-- Seed some education domain synonyms
INSERT INTO synonym_groups (id, canonical_term, synonyms) VALUES
    (uuid_generate_v4(), 'mathematics', ARRAY['math', 'maths', 'algebra', 'geometry', 'calculus', 'arithmetic']),
    (uuid_generate_v4(), 'science',     ARRAY['biology', 'chemistry', 'physics', 'natural science']),
    (uuid_generate_v4(), 'programming', ARRAY['coding', 'software development', 'computer science', 'development']),
    (uuid_generate_v4(), 'introduction', ARRAY['intro', 'beginner', 'basics', 'fundamentals', 'getting started']),
    (uuid_generate_v4(), 'advanced',    ARRAY['expert', 'senior', 'professional', 'in-depth']),
    (uuid_generate_v4(), 'tutorial',    ARRAY['guide', 'how-to', 'walkthrough', 'lesson', 'course']);

-- ============================================
-- Search: Terms (type-ahead)
-- ============================================

CREATE TABLE search_terms (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    term VARCHAR(200) UNIQUE NOT NULL,
    category VARCHAR(50) NOT NULL DEFAULT 'general',
    usage_count INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_search_terms_usage ON search_terms(usage_count DESC);
CREATE INDEX idx_search_terms_term_trgm ON search_terms USING gin(term gin_trgm_ops);

-- ============================================
-- Search: User Search History (last 20 per user)
-- ============================================

CREATE TABLE user_search_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    query VARCHAR(500) NOT NULL,
    filters_json JSONB NOT NULL DEFAULT '{}',
    searched_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_search_history_user_id ON user_search_history(user_id, searched_at DESC);

-- ============================================
-- Search: Index (tsvector + pinyin content)
-- ============================================

CREATE TABLE search_index (
    resource_id UUID PRIMARY KEY REFERENCES resources(id) ON DELETE CASCADE,
    tsvector_content TSVECTOR,
    pinyin_content TEXT NOT NULL DEFAULT '',
    tag_content TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_search_index_tsvector ON search_index USING gin(tsvector_content);
CREATE INDEX idx_search_index_pinyin_trgm ON search_index USING gin(pinyin_content gin_trgm_ops);

-- Trigger: keep search_index in sync with resources
CREATE OR REPLACE FUNCTION fn_update_search_index() RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO search_index (resource_id, tsvector_content, pinyin_content, tag_content, updated_at)
    VALUES (
        NEW.id,
        to_tsvector('english',
            COALESCE(NEW.title, '') || ' ' ||
            COALESCE(NEW.description, '') || ' ' ||
            COALESCE(NEW.content_body, '')
        ),
        '', -- pinyin content populated by Go service
        '', -- tag content populated by Go service
        NOW()
    )
    ON CONFLICT (resource_id) DO UPDATE SET
        tsvector_content = to_tsvector('english',
            COALESCE(NEW.title, '') || ' ' ||
            COALESCE(NEW.description, '') || ' ' ||
            COALESCE(NEW.content_body, '')
        ),
        updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_search_index_update
AFTER INSERT OR UPDATE OF title, description, content_body ON resources
FOR EACH ROW EXECUTE FUNCTION fn_update_search_index();

-- ============================================
-- Anomaly Flags (moderation)
-- ============================================

CREATE TABLE anomaly_flags (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    flag_type VARCHAR(20) NOT NULL CHECK (flag_type IN ('LIKE_RING', 'RATE_SPIKE', 'OTHER')),
    user_ids UUID[] NOT NULL DEFAULT '{}',
    evidence_json JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(10) NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'REVIEWED', 'DISMISSED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_anomaly_flags_status ON anomaly_flags(status);
CREATE INDEX idx_anomaly_flags_flag_type ON anomaly_flags(flag_type);

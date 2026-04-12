CREATE TABLE suppliers (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(200) NOT NULL,
    contact_json TEXT NOT NULL DEFAULT '',
    contact_mask TEXT NOT NULL DEFAULT '',
    tier VARCHAR(10) NOT NULL DEFAULT 'BRONZE' CHECK (tier IN ('BRONZE','SILVER','GOLD')),
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','SUSPENDED')),
    user_id UUID REFERENCES users(id),
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_suppliers_status ON suppliers(status);
CREATE INDEX idx_suppliers_user_id ON suppliers(user_id);

CREATE TABLE supplier_orders (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    supplier_id UUID NOT NULL REFERENCES suppliers(id),
    order_number VARCHAR(50) UNIQUE NOT NULL,
    order_lines JSONB NOT NULL DEFAULT '[]',
    status VARCHAR(20) NOT NULL DEFAULT 'CREATED' CHECK (status IN ('CREATED','CONFIRMED','SHIPPED','RECEIVED','QC_PASSED','QC_FAILED','CLOSED','CANCELLED')),
    delivery_date_confirmed TIMESTAMPTZ,
    delivery_date_confirmed_at TIMESTAMPTZ,
    received_at TIMESTAMPTZ,
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_supplier_orders_supplier_id ON supplier_orders(supplier_id);
CREATE INDEX idx_supplier_orders_status ON supplier_orders(status);
CREATE INDEX idx_supplier_orders_created_at ON supplier_orders(created_at);

CREATE TABLE supplier_asns (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_id UUID NOT NULL UNIQUE REFERENCES supplier_orders(id),
    tracking_info TEXT NOT NULL DEFAULT '',
    shipped_at TIMESTAMPTZ NOT NULL,
    expected_arrival TIMESTAMPTZ,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE supplier_qc_results (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_id UUID NOT NULL UNIQUE REFERENCES supplier_orders(id),
    inspected_units INT NOT NULL DEFAULT 0,
    defective_units INT NOT NULL DEFAULT 0,
    defect_rate_pct NUMERIC(5,2) NOT NULL DEFAULT 0,
    result VARCHAR(10) NOT NULL CHECK (result IN ('PASS','FAIL')),
    notes TEXT NOT NULL DEFAULT '',
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    submitted_by UUID NOT NULL REFERENCES users(id)
);

CREATE TABLE supplier_kpis (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    supplier_id UUID NOT NULL REFERENCES suppliers(id),
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    on_time_delivery_pct NUMERIC(5,2) NOT NULL DEFAULT 0,
    stockout_rate_pct NUMERIC(5,2) NOT NULL DEFAULT 0,
    return_rate_pct NUMERIC(5,2) NOT NULL DEFAULT 0,
    defect_rate_pct NUMERIC(5,2) NOT NULL DEFAULT 0,
    tier_assigned VARCHAR(10) NOT NULL DEFAULT 'BRONZE',
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_supplier_kpis_supplier_id ON supplier_kpis(supplier_id);

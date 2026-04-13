-- Enforce append-only semantics on audit_logs at the DB level.
-- Any UPDATE or DELETE on audit_logs raises an exception, preventing
-- even privileged writers from tampering with the compliance record.

CREATE OR REPLACE FUNCTION audit_logs_immutable()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_logs is append-only: % on row % is not allowed',
        TG_OP, OLD.id
        USING ERRCODE = 'restrict_violation';
END;
$$;

CREATE TRIGGER trg_audit_logs_no_update
    BEFORE UPDATE ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION audit_logs_immutable();

CREATE TRIGGER trg_audit_logs_no_delete
    BEFORE DELETE ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION audit_logs_immutable();

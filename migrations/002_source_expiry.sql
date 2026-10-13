-- Legacy records receive a bounded lifetime until a complete source refresh
-- supplies its actual schedule-derived expiry. Bootstrap all enabled sources.
UPDATE ip_reputation
SET expires_at = COALESCE(last_seen, first_seen, NOW()) + INTERVAL '48 hours'
WHERE expires_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_ip_reputation_expiry ON ip_reputation(expires_at);

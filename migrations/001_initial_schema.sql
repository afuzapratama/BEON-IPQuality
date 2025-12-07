-- BEON-IPQuality Database Schema
-- PostgreSQL with native inet/cidr support

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ==========================================
-- IP Reputation Table
-- ==========================================
CREATE TABLE IF NOT EXISTS ip_reputation (
    id BIGSERIAL PRIMARY KEY,
    
    -- IP range (single IP or CIDR) using native PostgreSQL type
    ip_start INET NOT NULL,
    ip_end INET NOT NULL,
    cidr CIDR,  -- Original CIDR notation if applicable
    
    -- Source information
    source VARCHAR(100) NOT NULL,
    source_name VARCHAR(255),
    
    -- Threat classification
    threat_type VARCHAR(50) NOT NULL,
    -- Types: tor, vpn, proxy, datacenter, botnet_c2, malware, spam, hijacked, attack, suspicious
    
    -- Confidence score (0.0 to 1.0)
    confidence DECIMAL(4,3) NOT NULL DEFAULT 1.0,
    
    -- Weight for scoring (0 to 100)
    weight INTEGER NOT NULL DEFAULT 50,
    
    -- Timestamps
    first_seen TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_seen TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE,
    
    -- Additional metadata (JSON)
    metadata JSONB DEFAULT '{}',
    
    -- Constraints
    CONSTRAINT valid_confidence CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT valid_weight CHECK (weight >= 0 AND weight <= 100)
);

-- Create unique index on ip range + source
CREATE UNIQUE INDEX IF NOT EXISTS idx_ip_reputation_unique 
    ON ip_reputation(ip_start, ip_end, source);

-- Indexes for fast lookups using GiST for inet ranges
CREATE INDEX IF NOT EXISTS idx_ip_reputation_ip_start ON ip_reputation(ip_start);
CREATE INDEX IF NOT EXISTS idx_ip_reputation_ip_end ON ip_reputation(ip_end);
CREATE INDEX IF NOT EXISTS idx_ip_reputation_cidr ON ip_reputation USING gist(cidr inet_ops);
CREATE INDEX IF NOT EXISTS idx_ip_reputation_source ON ip_reputation(source);
CREATE INDEX IF NOT EXISTS idx_ip_reputation_threat_type ON ip_reputation(threat_type);
CREATE INDEX IF NOT EXISTS idx_ip_reputation_last_seen ON ip_reputation(last_seen DESC);
CREATE INDEX IF NOT EXISTS idx_ip_reputation_expires_at ON ip_reputation(expires_at) WHERE expires_at IS NOT NULL;

-- ==========================================
-- Feed Sources Table
-- ==========================================
CREATE TABLE IF NOT EXISTS feed_sources (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    display_name VARCHAR(255),
    description TEXT,
    url VARCHAR(1024),
    format VARCHAR(50),
    threat_type VARCHAR(50),
    default_confidence DECIMAL(4,3) DEFAULT 0.8,
    default_weight INTEGER DEFAULT 50,
    update_frequency INTERVAL DEFAULT '1 hour',
    enabled BOOLEAN DEFAULT true,
    last_fetch TIMESTAMP WITH TIME ZONE,
    last_success TIMESTAMP WITH TIME ZONE,
    fetch_count INTEGER DEFAULT 0,
    error_count INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- ==========================================
-- ASN Information Table
-- ==========================================
CREATE TABLE IF NOT EXISTS asn_info (
    asn INTEGER PRIMARY KEY,
    name VARCHAR(255),
    org VARCHAR(255),
    country_code CHAR(2),
    asn_type VARCHAR(50), -- datacenter, isp, business, education, government
    risk_modifier INTEGER DEFAULT 0, -- bonus/penalty for scoring
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_asn_info_type ON asn_info(asn_type);
CREATE INDEX IF NOT EXISTS idx_asn_info_country ON asn_info(country_code);

-- ==========================================
-- Whitelist Table
-- ==========================================
CREATE TABLE IF NOT EXISTS whitelist (
    id SERIAL PRIMARY KEY,
    ip_start INET NOT NULL,
    ip_end INET NOT NULL,
    cidr CIDR,
    name VARCHAR(255),
    description TEXT,
    source VARCHAR(100),
    permanent BOOLEAN DEFAULT false,
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_whitelist_unique ON whitelist(ip_start, ip_end);
CREATE INDEX IF NOT EXISTS idx_whitelist_cidr ON whitelist USING gist(cidr inet_ops);

-- ==========================================
-- API Keys Table
-- ==========================================
CREATE TABLE IF NOT EXISTS api_keys (
    id SERIAL PRIMARY KEY,
    key_hash VARCHAR(64) NOT NULL UNIQUE, -- SHA-256 hash of the API key
    key_prefix VARCHAR(12), -- First 8 chars for identification (beon_xxxx)
    name VARCHAR(255),
    description TEXT,
    tier VARCHAR(50) DEFAULT 'free', -- free, basic, premium, enterprise
    rate_limit INTEGER DEFAULT 1000, -- requests per minute
    daily_limit INTEGER DEFAULT 10000, -- requests per day
    enabled BOOLEAN DEFAULT true,
    ip_whitelist TEXT[], -- Array of allowed IPs (null = any)
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE,
    last_used TIMESTAMP WITH TIME ZONE,
    total_requests BIGINT DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(key_prefix);

-- ==========================================
-- API Request Log Table
-- ==========================================
CREATE TABLE IF NOT EXISTS api_request_log (
    id BIGSERIAL PRIMARY KEY,
    api_key_id INTEGER REFERENCES api_keys(id),
    ip_queried INET NOT NULL,
    risk_score INTEGER,
    response_time_ms DECIMAL(10,3),
    cached BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_api_log_created ON api_request_log(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_api_log_ip ON api_request_log(ip_queried);

-- ==========================================
-- Feed Fetch History Table
-- ==========================================
CREATE TABLE IF NOT EXISTS feed_fetch_history (
    id BIGSERIAL PRIMARY KEY,
    feed_name VARCHAR(100) NOT NULL,
    source_url VARCHAR(1024),
    status VARCHAR(20), -- success, error, partial
    entries_count INTEGER DEFAULT 0,
    new_entries INTEGER DEFAULT 0,
    updated_entries INTEGER DEFAULT 0,
    duration_ms INTEGER,
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_feed_history_name ON feed_fetch_history(feed_name);
CREATE INDEX IF NOT EXISTS idx_feed_history_created ON feed_fetch_history(created_at DESC);

-- ==========================================
-- MMDB Compilation History
-- ==========================================
CREATE TABLE IF NOT EXISTS mmdb_compile_history (
    id SERIAL PRIMARY KEY,
    filename VARCHAR(255) NOT NULL,
    file_size BIGINT,
    entries_count INTEGER DEFAULT 0,
    ipv4_entries INTEGER DEFAULT 0,
    ipv6_entries INTEGER DEFAULT 0,
    compile_duration_ms INTEGER,
    status VARCHAR(20), -- success, error
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- ==========================================
-- Functions
-- ==========================================

-- Function to check if an IP is in the reputation table
CREATE OR REPLACE FUNCTION check_ip_reputation(check_ip INET)
RETURNS TABLE (
    source VARCHAR(100),
    threat_type VARCHAR(50),
    confidence DECIMAL(4,3),
    weight INTEGER,
    first_seen TIMESTAMP WITH TIME ZONE,
    last_seen TIMESTAMP WITH TIME ZONE
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        r.source,
        r.threat_type,
        r.confidence,
        r.weight,
        r.first_seen,
        r.last_seen
    FROM ip_reputation r
    WHERE check_ip >= r.ip_start 
      AND check_ip <= r.ip_end
      AND (r.expires_at IS NULL OR r.expires_at > NOW())
    ORDER BY r.weight DESC, r.confidence DESC;
END;
$$ LANGUAGE plpgsql;

-- Function to check if IP is whitelisted
CREATE OR REPLACE FUNCTION is_whitelisted(check_ip INET)
RETURNS BOOLEAN AS $$
BEGIN
    RETURN EXISTS (
        SELECT 1 FROM whitelist w
        WHERE check_ip >= w.ip_start 
          AND check_ip <= w.ip_end
          AND (w.permanent = true OR w.expires_at IS NULL OR w.expires_at > NOW())
    );
END;
$$ LANGUAGE plpgsql;

-- Function to insert or update IP reputation entry
CREATE OR REPLACE FUNCTION upsert_ip_reputation(
    p_ip_start INET,
    p_ip_end INET,
    p_cidr CIDR,
    p_source VARCHAR(100),
    p_source_name VARCHAR(255),
    p_threat_type VARCHAR(50),
    p_confidence DECIMAL(4,3),
    p_weight INTEGER,
    p_expires_at TIMESTAMP WITH TIME ZONE DEFAULT NULL,
    p_metadata JSONB DEFAULT '{}'
) RETURNS BIGINT AS $$
DECLARE
    v_id BIGINT;
BEGIN
    INSERT INTO ip_reputation (
        ip_start, ip_end, cidr, source, source_name, 
        threat_type, confidence, weight, expires_at, metadata, last_seen
    ) VALUES (
        p_ip_start, p_ip_end, p_cidr, p_source, p_source_name,
        p_threat_type, p_confidence, p_weight, p_expires_at, p_metadata, NOW()
    )
    ON CONFLICT (ip_start, ip_end, source) 
    DO UPDATE SET
        confidence = EXCLUDED.confidence,
        weight = EXCLUDED.weight,
        last_seen = NOW(),
        expires_at = EXCLUDED.expires_at,
        metadata = EXCLUDED.metadata
    RETURNING id INTO v_id;
    
    RETURN v_id;
END;
$$ LANGUAGE plpgsql;

-- Function to clean expired entries
CREATE OR REPLACE FUNCTION cleanup_expired_entries()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM ip_reputation 
    WHERE expires_at IS NOT NULL AND expires_at < NOW();
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    
    DELETE FROM whitelist 
    WHERE permanent = false AND expires_at IS NOT NULL AND expires_at < NOW();
    
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Function to get statistics
CREATE OR REPLACE FUNCTION get_reputation_stats()
RETURNS TABLE (
    total_entries BIGINT,
    unique_sources BIGINT,
    threat_type_counts JSONB,
    oldest_entry TIMESTAMP WITH TIME ZONE,
    newest_entry TIMESTAMP WITH TIME ZONE
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        COUNT(*)::BIGINT as total_entries,
        COUNT(DISTINCT source)::BIGINT as unique_sources,
        jsonb_object_agg(tt.threat_type, tt.cnt) as threat_type_counts,
        MIN(first_seen) as oldest_entry,
        MAX(last_seen) as newest_entry
    FROM ip_reputation,
    LATERAL (
        SELECT threat_type, COUNT(*) as cnt 
        FROM ip_reputation 
        GROUP BY threat_type
    ) tt
    GROUP BY tt.threat_type, tt.cnt;
END;
$$ LANGUAGE plpgsql;

-- ==========================================
-- Insert Default Feed Sources
-- ==========================================
INSERT INTO feed_sources (name, display_name, description, url, format, threat_type, default_confidence, default_weight, update_frequency) VALUES
    ('tor_exit_nodes', 'Tor Exit Nodes', 'Official Tor exit node list', 'https://check.torproject.org/torbulkexitlist', 'plain', 'tor', 0.95, 80, '1 hour'),
    ('tor_dan_exit', 'Dan.me.uk Tor Exits', 'Dan''s Tor exit list', 'https://www.dan.me.uk/torlist/?exit', 'plain', 'tor', 0.90, 80, '1 hour'),
    ('spamhaus_drop', 'Spamhaus DROP', 'Don''t Route Or Peer list', 'https://www.spamhaus.org/drop/drop.txt', 'cidr', 'hijacked', 0.95, 90, '12 hours'),
    ('spamhaus_edrop', 'Spamhaus EDROP', 'Extended DROP list', 'https://www.spamhaus.org/drop/edrop.txt', 'cidr', 'hijacked', 0.95, 90, '12 hours'),
    ('firehol_level1', 'FireHOL Level 1', 'FireHOL blocklist level 1', 'https://raw.githubusercontent.com/firehol/blocklist-ipsets/master/firehol_level1.netset', 'netset', 'attack', 0.85, 70, '6 hours'),
    ('firehol_level2', 'FireHOL Level 2', 'FireHOL blocklist level 2', 'https://raw.githubusercontent.com/firehol/blocklist-ipsets/master/firehol_level2.netset', 'netset', 'attack', 0.80, 60, '6 hours'),
    ('abuse_feodo', 'Abuse.ch Feodo', 'Feodo botnet C2 tracker', 'https://feodotracker.abuse.ch/downloads/ipblocklist.txt', 'plain', 'botnet_c2', 0.95, 95, '1 hour'),
    ('abuse_sslbl', 'Abuse.ch SSL BL', 'SSL Blacklist', 'https://sslbl.abuse.ch/blacklist/sslipblacklist.txt', 'plain', 'malware', 0.90, 85, '1 hour'),
    ('blocklist_de', 'Blocklist.de', 'Fail2ban reported IPs', 'https://lists.blocklist.de/lists/all.txt', 'plain', 'attack', 0.70, 50, '1 hour'),
    ('emergingthreats', 'Emerging Threats', 'ET compromised IPs', 'https://rules.emergingthreats.net/blockrules/compromised-ips.txt', 'plain', 'malware', 0.85, 75, '6 hours'),
    ('cinsscore', 'CI Army', 'CI Bad Guys list', 'https://cinsscore.com/list/ci-badguys.txt', 'plain', 'attack', 0.75, 55, '1 hour'),
    ('greensnow', 'GreenSnow', 'GreenSnow blocklist', 'https://blocklist.greensnow.co/greensnow.txt', 'plain', 'attack', 0.70, 50, '1 hour')
ON CONFLICT (name) DO NOTHING;

-- ==========================================
-- Insert Known Datacenter ASNs (sample)
-- ==========================================
INSERT INTO asn_info (asn, name, org, country_code, asn_type, risk_modifier) VALUES
    (14061, 'DIGITALOCEAN-ASN', 'DigitalOcean, LLC', 'US', 'datacenter', 15),
    (16509, 'AMAZON-02', 'Amazon.com, Inc.', 'US', 'datacenter', 15),
    (15169, 'GOOGLE', 'Google LLC', 'US', 'datacenter', 10),
    (8075, 'MICROSOFT-CORP-MSN-AS-BLOCK', 'Microsoft Corporation', 'US', 'datacenter', 10),
    (13335, 'CLOUDFLARENET', 'Cloudflare, Inc.', 'US', 'datacenter', 10),
    (20473, 'AS-CHOOPA', 'Vultr Holdings, LLC', 'US', 'datacenter', 20),
    (63949, 'LINODE-AP', 'Akamai Connected Cloud', 'US', 'datacenter', 15),
    (24940, 'HETZNER-AS', 'Hetzner Online GmbH', 'DE', 'datacenter', 15),
    (16276, 'OVH', 'OVH SAS', 'FR', 'datacenter', 15),
    (14618, 'AMAZON-AES', 'Amazon.com, Inc.', 'US', 'datacenter', 15)
ON CONFLICT (asn) DO NOTHING;

-- Grant permissions (adjust as needed)
-- GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO beon;
-- GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO beon;

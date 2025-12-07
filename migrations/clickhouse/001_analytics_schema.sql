-- ClickHouse Schema for BEON-IPQuality Analytics
-- Run this after ClickHouse container is up

-- Database
CREATE DATABASE IF NOT EXISTS ipquality;

-- API Request Logs (for analytics)
CREATE TABLE IF NOT EXISTS ipquality.api_requests (
    timestamp DateTime DEFAULT now(),
    request_id UUID DEFAULT generateUUIDv4(),
    ip_checked String,
    client_ip String,
    api_key String DEFAULT '',
    endpoint String,
    method String,
    
    -- Result data
    risk_score UInt8,
    risk_level LowCardinality(String),
    is_proxy Bool DEFAULT false,
    is_vpn Bool DEFAULT false,
    is_tor Bool DEFAULT false,
    is_datacenter Bool DEFAULT false,
    is_botnet Bool DEFAULT false,
    
    -- Geo data
    country_code LowCardinality(String) DEFAULT '',
    country String DEFAULT '',
    city String DEFAULT '',
    asn UInt32 DEFAULT 0,
    asn_org String DEFAULT '',
    
    -- Performance
    query_time_ms Float32,
    cached Bool DEFAULT false,
    
    -- Request metadata
    user_agent String DEFAULT '',
    response_code UInt16
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (timestamp, ip_checked)
TTL timestamp + INTERVAL 90 DAY;

-- Aggregated stats per hour
CREATE TABLE IF NOT EXISTS ipquality.hourly_stats (
    hour DateTime,
    total_requests UInt64,
    unique_ips UInt64,
    avg_query_time Float32,
    cache_hit_rate Float32,
    
    -- Risk distribution
    clean_count UInt64,
    low_risk_count UInt64,
    medium_risk_count UInt64,
    high_risk_count UInt64,
    critical_count UInt64,
    
    -- Threat type counts
    proxy_count UInt64,
    vpn_count UInt64,
    tor_count UInt64,
    datacenter_count UInt64,
    botnet_count UInt64,
    
    -- Top countries
    top_countries Array(Tuple(String, UInt64))
) ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(hour)
ORDER BY hour;

-- Materialized view for hourly aggregation
CREATE MATERIALIZED VIEW IF NOT EXISTS ipquality.hourly_stats_mv
TO ipquality.hourly_stats
AS SELECT
    toStartOfHour(timestamp) AS hour,
    count() AS total_requests,
    uniq(ip_checked) AS unique_ips,
    avg(query_time_ms) AS avg_query_time,
    countIf(cached) / count() * 100 AS cache_hit_rate,
    
    countIf(risk_level = 'clean') AS clean_count,
    countIf(risk_level = 'low') AS low_risk_count,
    countIf(risk_level = 'medium') AS medium_risk_count,
    countIf(risk_level = 'high') AS high_risk_count,
    countIf(risk_level = 'critical') AS critical_count,
    
    countIf(is_proxy) AS proxy_count,
    countIf(is_vpn) AS vpn_count,
    countIf(is_tor) AS tor_count,
    countIf(is_datacenter) AS datacenter_count,
    countIf(is_botnet) AS botnet_count,
    
    topK(10)(country_code) AS top_countries
FROM ipquality.api_requests
GROUP BY hour;

-- Daily summary table
CREATE TABLE IF NOT EXISTS ipquality.daily_summary (
    date Date,
    total_requests UInt64,
    unique_ips UInt64,
    unique_api_keys UInt64,
    avg_response_time Float32,
    p95_response_time Float32,
    p99_response_time Float32,
    error_rate Float32,
    cache_hit_rate Float32
) ENGINE = SummingMergeTree()
ORDER BY date;

-- Top threats table (for dashboard)
CREATE TABLE IF NOT EXISTS ipquality.top_threats (
    date Date,
    ip String,
    risk_score UInt8,
    threat_types Array(String),
    country_code String,
    asn UInt32,
    asn_org String,
    hit_count UInt64
) ENGINE = ReplacingMergeTree(hit_count)
PARTITION BY toYYYYMM(date)
ORDER BY (date, ip);

-- Scan results log (from Judge Node)
CREATE TABLE IF NOT EXISTS ipquality.scan_results (
    timestamp DateTime DEFAULT now(),
    ip String,
    is_proxy Bool,
    is_socks4 Bool,
    is_socks5 Bool,
    is_http_proxy Bool,
    is_http_connect Bool,
    open_ports Array(UInt16),
    proxy_ports Array(UInt16),
    scan_time_ms Float32
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (timestamp, ip)
TTL timestamp + INTERVAL 30 DAY;

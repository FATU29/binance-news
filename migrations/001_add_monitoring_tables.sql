-- Migration: Add structure monitoring tables
-- Created: 2025-12-30

-- Table for tracking selector patterns
CREATE TABLE IF NOT EXISTS selector_patterns (
    id SERIAL PRIMARY KEY,
    source VARCHAR(100) NOT NULL,
    element_type VARCHAR(50) NOT NULL,
    selector VARCHAR(500) NOT NULL,
    success_count INT DEFAULT 0,
    failure_count INT DEFAULT 0,
    success_rate FLOAT DEFAULT 0,
    last_used TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- Indexes
    CONSTRAINT unique_selector UNIQUE (source, element_type, selector)
);

CREATE INDEX idx_selector_patterns_source ON selector_patterns(source, element_type);
CREATE INDEX idx_selector_patterns_last_used ON selector_patterns(last_used);
CREATE INDEX idx_selector_patterns_success_rate ON selector_patterns(success_rate DESC);

-- Table for structure change alerts
CREATE TABLE IF NOT EXISTS structure_alerts (
    id SERIAL PRIMARY KEY,
    source VARCHAR(100) NOT NULL,
    selector VARCHAR(500),
    failure_count INT DEFAULT 0,
    severity VARCHAR(20) CHECK (severity IN ('warning', 'critical')),
    notified BOOLEAN DEFAULT FALSE,
    resolved BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT NOW(),
    resolved_at TIMESTAMP,
    
    -- Indexes
    CONSTRAINT check_severity CHECK (severity IN ('warning', 'critical'))
);

CREATE INDEX idx_structure_alerts_source ON structure_alerts(source);
CREATE INDEX idx_structure_alerts_resolved ON structure_alerts(resolved) WHERE resolved = FALSE;
CREATE INDEX idx_structure_alerts_created ON structure_alerts(created_at DESC);
CREATE INDEX idx_structure_alerts_severity ON structure_alerts(severity);

-- Comments
COMMENT ON TABLE selector_patterns IS 'Tracks selector usage patterns and success rates for adaptive learning';
COMMENT ON TABLE structure_alerts IS 'Alerts when website structure changes are detected';

COMMENT ON COLUMN selector_patterns.success_rate IS 'Percentage of successful extractions (0-100)';
COMMENT ON COLUMN structure_alerts.severity IS 'Alert severity level: warning or critical';


-- Migration for selector learning and adaptive crawling features
-- Run: psql -U postgres -d crypto_news -f 002_add_selector_learning.sql

-- Table for learned selectors
CREATE TABLE IF NOT EXISTS learned_selectors (
    id SERIAL PRIMARY KEY,
    source VARCHAR(100) NOT NULL,
    element_type VARCHAR(50) NOT NULL,
    selector VARCHAR(500) NOT NULL,
    confidence FLOAT DEFAULT 0,
    success_count INT DEFAULT 0,
    failure_count INT DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    is_primary BOOLEAN DEFAULT FALSE,
    discovered_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_tested TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    CONSTRAINT unique_source_element_selector UNIQUE (source, element_type, selector)
);

CREATE INDEX idx_learned_selectors_source ON learned_selectors(source);
CREATE INDEX idx_learned_selectors_element_type ON learned_selectors(element_type);
CREATE INDEX idx_learned_selectors_confidence ON learned_selectors(confidence DESC);
CREATE INDEX idx_learned_selectors_active ON learned_selectors(is_active);
CREATE INDEX idx_learned_selectors_primary ON learned_selectors(is_primary);

-- Table for selector fallbacks
CREATE TABLE IF NOT EXISTS selector_fallbacks (
    id SERIAL PRIMARY KEY,
    source VARCHAR(100) NOT NULL,
    element_type VARCHAR(50) NOT NULL,
    primary_selector VARCHAR(500) NOT NULL,
    fallback_selector VARCHAR(500) NOT NULL,
    priority INT DEFAULT 0,
    success_rate FLOAT DEFAULT 0,
    last_used TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_selector_fallbacks_source ON selector_fallbacks(source);
CREATE INDEX idx_selector_fallbacks_element_type ON selector_fallbacks(element_type);
CREATE INDEX idx_selector_fallbacks_priority ON selector_fallbacks(priority);

-- Table for crawl source configurations (dynamic source management)
CREATE TABLE IF NOT EXISTS crawl_sources (
    id VARCHAR(100) PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    base_url VARCHAR(500) NOT NULL,
    enabled BOOLEAN DEFAULT TRUE,
    priority INT DEFAULT 0,
    crawl_freq INT DEFAULT 60, -- minutes
    last_crawled TIMESTAMP,
    selectors JSONB,
    categories JSONB,
    language VARCHAR(10) DEFAULT 'en',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
);

CREATE INDEX idx_crawl_sources_name ON crawl_sources(name);
CREATE INDEX idx_crawl_sources_enabled ON crawl_sources(enabled);
CREATE INDEX idx_crawl_sources_priority ON crawl_sources(priority DESC);
CREATE INDEX idx_crawl_sources_last_crawled ON crawl_sources(last_crawled);
CREATE INDEX idx_crawl_sources_deleted_at ON crawl_sources(deleted_at);

-- Enhance existing selector_patterns table (if not already has these columns)
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name='selector_patterns' AND column_name='confidence') THEN
        ALTER TABLE selector_patterns ADD COLUMN confidence FLOAT DEFAULT 0;
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name='selector_patterns' AND column_name='is_active') THEN
        ALTER TABLE selector_patterns ADD COLUMN is_active BOOLEAN DEFAULT TRUE;
    END IF;
END $$;

-- Enhance existing structure_alerts table
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name='structure_alerts' AND column_name='auto_healed') THEN
        ALTER TABLE structure_alerts ADD COLUMN auto_healed BOOLEAN DEFAULT FALSE;
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name='structure_alerts' AND column_name='replacement_selector') THEN
        ALTER TABLE structure_alerts ADD COLUMN replacement_selector VARCHAR(500);
    END IF;
END $$;

-- Insert default sources into crawl_sources table
INSERT INTO crawl_sources (id, name, base_url, enabled, priority, crawl_freq, selectors, language)
VALUES 
    ('src-cointelegraph', 'cointelegraph', 'https://cointelegraph.com', TRUE, 10, 30, 
     '{"title":"h1.post__title","content":".post-content","summary":".post__lead","author":".post-meta__author-name","published_at":".post-meta__publish-date","image_url":".post__picture img","article_list":".post-card-inline","article_link":"a.post-card-inline__title-link"}'::jsonb, 'en'),
    
    ('src-coindesk', 'coindesk', 'https://www.coindesk.com', TRUE, 9, 30,
     '{"title":"h1","content":".article-body","author":".author-name","published_at":"time","image_url":"img","article_list":".article-cardstyles__StyledWrapper","article_link":"a"}'::jsonb, 'en'),
    
    ('src-cryptonews', 'cryptonews', 'https://cryptonews.com', TRUE, 8, 45,
     '{"title":"h1.article__title","content":".article__content","author":".article__author","published_at":".article__date","image_url":".article__image img","article_list":".article-item","article_link":"a.article-item__link"}'::jsonb, 'en'),
    
    ('src-binance', 'binance', 'https://www.binance.com', TRUE, 10, 30, '{}'::jsonb, 'en'),
    
    ('src-coinmarketcap', 'coinmarketcap', 'https://coinmarketcap.com', TRUE, 7, 60,
     '{"title":"h1","content":".content","article_list":".sc-aef7b723-0","article_link":"a"}'::jsonb, 'en'),
    
    ('src-bitcoincom', 'bitcoincom', 'https://news.bitcoin.com', TRUE, 8, 45, '{}'::jsonb, 'en'),
    
    ('src-theblock', 'theblock', 'https://www.theblock.co', TRUE, 9, 30, '{}'::jsonb, 'en'),
    
    ('src-decrypt', 'decrypt', 'https://decrypt.co', TRUE, 7, 60, '{}'::jsonb, 'en'),
    
    ('src-utoday', 'utoday', 'https://u.today', TRUE, 6, 60, '{}'::jsonb, 'en'),
    
    ('src-cryptoslate', 'cryptoslate', 'https://cryptoslate.com', TRUE, 7, 45, '{}'::jsonb, 'en')
ON CONFLICT (id) DO NOTHING;

-- Add comments for documentation
COMMENT ON TABLE learned_selectors IS 'Stores automatically discovered and learned CSS selectors for web scraping';
COMMENT ON TABLE selector_fallbacks IS 'Stores fallback selector strategies when primary selectors fail';
COMMENT ON TABLE crawl_sources IS 'Dynamic configuration for news crawl sources';

COMMENT ON COLUMN learned_selectors.confidence IS 'Confidence score 0-100 based on success rate';
COMMENT ON COLUMN learned_selectors.is_primary IS 'Whether this is the primary selector for this element type';
COMMENT ON COLUMN selector_fallbacks.priority IS 'Lower number = higher priority';
COMMENT ON COLUMN crawl_sources.crawl_freq IS 'Crawl frequency in minutes';

-- Create a view for selector health monitoring
CREATE OR REPLACE VIEW selector_health_view AS
SELECT 
    ls.source,
    ls.element_type,
    ls.selector,
    ls.confidence,
    ls.success_count,
    ls.failure_count,
    CASE 
        WHEN ls.success_count + ls.failure_count > 0 
        THEN (ls.success_count::FLOAT / (ls.success_count + ls.failure_count) * 100)
        ELSE 0 
    END as actual_success_rate,
    ls.is_active,
    ls.is_primary,
    ls.last_tested,
    CASE
        WHEN ls.confidence > 80 AND ls.is_active THEN 'healthy'
        WHEN ls.confidence > 50 AND ls.is_active THEN 'warning'
        WHEN ls.is_active THEN 'critical'
        ELSE 'inactive'
    END as health_status
FROM learned_selectors ls
ORDER BY ls.source, ls.element_type, ls.confidence DESC;

COMMENT ON VIEW selector_health_view IS 'Health monitoring view for learned selectors';

-- Create function to auto-cleanup old inactive selectors
CREATE OR REPLACE FUNCTION cleanup_old_selectors()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM learned_selectors
    WHERE is_active = FALSE
      AND last_tested < NOW() - INTERVAL '90 days'
      AND success_count = 0;
    
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION cleanup_old_selectors IS 'Cleanup selectors inactive for 90+ days with no successes';

CREATE TABLE IF NOT EXISTS ref_countries_iso3166_1 (
	alphabetic_code_2   VARCHAR(2),
	alphabetic_code_3   VARCHAR(3),
	numeric_code        INT2,
	country_name        TEXT,
	official_state_name TEXT,
	sovereignty         TEXT,
	top_domain          TEXT,
	PRIMARY KEY(alphabetic_code_2)
);
COMMENT ON TABLE ref_countries_iso3166_1 IS 'ISO 3166-1 codes list, REF: https://en.wikipedia.org/wiki/List_of_ISO_3166_country_codes';

INSERT INTO ref_countries_iso3166_1 (alphabetic_code_2, alphabetic_code_3, numeric_code, country_name, official_state_name, sovereignty, top_domain) VALUES
    ('US', 'USA', 840, 'United States', 'United States of America', 'UN Member State', 'us'),
    ('TW', 'TWN', 158, 'Taiwan', 'Republic of China', 'Disputed', 'tw'),
    ('PY', 'PRY', 600, 'Paraguay', 'Republic of Paraguay', 'UN member state', 'py');

CREATE TABLE IF NOT EXISTS ref_languages_iso639_1 (
	alphabetic_code VARCHAR(2),
	language_name   TEXT,
	language_name_en TEXT,
	PRIMARY KEY(alphabetic_code)
);
COMMENT ON TABLE ref_languages_iso639_1 IS 'ISO 639-1 codes list, REF: https://en.wikipedia.org/wiki/List_of_ISO_639-1_codes';

INSERT INTO ref_languages_iso639_1 (alphabetic_code, language_name, language_name_en) VALUES
    ('en', 'English', 'English'),
    ('zh', '中文', 'Chinese'),
    ('es', 'Español', 'Spanish');

CREATE TABLE IF NOT EXISTS ref_currencies_iso4217 (
	alphabetic_code VARCHAR(3),
	numeric_code    INT2,
	minor_unit      INT2,
	currency_name   TEXT,
	PRIMARY KEY(alphabetic_code)
);
COMMENT ON TABLE ref_currencies_iso4217 IS 'ISO 4217 codes list, REF: https://en.wikipedia.org/wiki/ISO_4217';

INSERT INTO ref_currencies_iso4217 (alphabetic_code, numeric_code, minor_unit, currency_name) VALUES
    ('USD', 840, 2, 'US Dollar'),
    ('TWD', 901, 2, 'New Taiwan Dollar'),
    ('PYG', 600, 0, 'Paraguayan Guaraní');

-- Create roles table
CREATE TABLE roles (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NULL
);

-- Create permissions table
CREATE TABLE permissions (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    resource VARCHAR(100) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create users table
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(50) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    is_active BOOLEAN DEFAULT true,
    email_verified BOOLEAN DEFAULT false,
    last_login TIMESTAMP WITH TIME ZONE,
    main_country    VARCHAR(2) NOT NULL DEFAULT 'US',
	main_language   VARCHAR(2) NOT NULL DEFAULT 'en',
	main_currency   VARCHAR(3) NOT NULL DEFAULT 'USD',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NULL,
    CONSTRAINT fk_users_main_country FOREIGN KEY (main_country) REFERENCES ref_countries_iso3166_1(alphabetic_code_2),
    CONSTRAINT fk_users_main_language FOREIGN KEY (main_language) REFERENCES ref_languages_iso639_1(alphabetic_code),
    CONSTRAINT fk_users_main_currency FOREIGN KEY (main_currency) REFERENCES ref_currencies_iso4217(alphabetic_code)
);

-- Create user_roles junction table
CREATE TABLE user_roles (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, role_id)
);

-- Create role_permissions junction table
CREATE TABLE role_permissions (
    id SERIAL PRIMARY KEY,
    role_id INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id INTEGER NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(role_id, permission_id)
);

-- Create indexes for better performance
CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_active ON users(is_active);
CREATE INDEX idx_user_roles_user_id ON user_roles(user_id);
CREATE INDEX idx_user_roles_role_id ON user_roles(role_id);
CREATE INDEX idx_permissions_name ON permissions(name);
CREATE INDEX idx_permissions_resource ON permissions(resource);
CREATE INDEX idx_role_permissions_role_id ON role_permissions(role_id);
CREATE INDEX idx_role_permissions_permission_id ON role_permissions(permission_id);

-- Insert default roles
INSERT INTO roles (name, description) VALUES
    ('admin', 'Administrator with full access'),
    ('user', 'Regular user with basic access');

-- Insert default permissions
INSERT INTO permissions (name, description, resource) VALUES
    ('ledger.manage', 'Manage ledger entries', 'ledger/manage'),
    ('ledger.reports', 'View ledger reports', 'ledger/reports'),
    ('system.users', 'Manage users and their roles', 'system/users'),
    ('system.config', 'Manage system settings', 'system/config');

-- Assign permissions to admin role (all permissions)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'admin';

-- Assign basic permissions to user role
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r, permissions p
WHERE r.name = 'user'
AND p.name IN ('ledger.manage', 'ledger.reports');

-- Create trigger function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create triggers for updated_at
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_roles_updated_at BEFORE UPDATE ON roles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_permissions_updated_at BEFORE UPDATE ON permissions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

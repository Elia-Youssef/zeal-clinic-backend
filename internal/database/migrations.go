package database

const schema = `
-- ============================================================
-- SYSTEM TABLES
-- ============================================================

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name  TEXT NOT NULL DEFAULT '',
    role          TEXT NOT NULL DEFAULT 'user' CHECK(role IN ('super-admin','admin','user')),
    is_active     INTEGER NOT NULL DEFAULT 1,
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS roles (
    name        TEXT PRIMARY KEY,
    label       TEXT NOT NULL,
    scopes      TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS tokens (
    id         TEXT PRIMARY KEY,
    token      TEXT NOT NULL UNIQUE,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS audit_log (
    id          TEXT PRIMARY KEY,
    user_name   TEXT NOT NULL DEFAULT '',
    user_role   TEXT NOT NULL DEFAULT '',
    action      TEXT NOT NULL CHECK(action IN ('create','update','delete')),
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    details     TEXT NOT NULL DEFAULT '',
    ip_address  TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- ROOMS
-- ============================================================

CREATE TABLE IF NOT EXISTS rooms (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    type         TEXT NOT NULL CHECK(type IN ('Consultation','Procedure','General','Hospital')),
    is_available INTEGER NOT NULL DEFAULT 1,
    created_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- ALLERGIES
-- ============================================================

CREATE TABLE IF NOT EXISTS allergies (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- PROCEDURE TYPES
-- ============================================================

CREATE TABLE IF NOT EXISTS procedure_types (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- PROCEDURE CATEGORIES
-- ============================================================

CREATE TABLE IF NOT EXISTS procedure_categories (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    parent_id   TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- PROCEDURES
-- ============================================================

CREATE TABLE IF NOT EXISTS procedures (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    type_id         TEXT NOT NULL DEFAULT '',
    category_id     TEXT NOT NULL DEFAULT '',
    price           REAL NOT NULL DEFAULT 0,
    price_note      TEXT NOT NULL DEFAULT '',
    is_active       INTEGER NOT NULL DEFAULT 1,
    remarks         TEXT NOT NULL DEFAULT '',
    includes        TEXT NOT NULL DEFAULT '[]',
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS procedure_sessions (
    id              TEXT PRIMARY KEY,
    procedure_id    TEXT NOT NULL REFERENCES procedures(id) ON DELETE CASCADE,
    session_number  INTEGER NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    price           REAL NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS procedure_allergy_conflicts (
    id           TEXT PRIMARY KEY,
    procedure_id TEXT NOT NULL REFERENCES procedures(id) ON DELETE CASCADE,
    allergy_id   TEXT NOT NULL REFERENCES allergies(id) ON DELETE CASCADE,
    notes        TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(procedure_id, allergy_id)
);

-- ============================================================
-- MEDICINES
-- ============================================================

CREATE TABLE IF NOT EXISTS medicines (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- PATIENTS
-- ============================================================

CREATE TABLE IF NOT EXISTS patients (
    id                      TEXT PRIMARY KEY,
    first_name              TEXT NOT NULL,
    middle_name             TEXT NOT NULL DEFAULT '',
    last_name               TEXT NOT NULL,
    gender                  TEXT NOT NULL CHECK(gender IN ('Male','Female')),
    date_of_birth           TEXT NOT NULL,
    contact                 TEXT NOT NULL,
    email                   TEXT NOT NULL DEFAULT '',
    emergency_contact_name  TEXT NOT NULL DEFAULT '',
    emergency_contact_phone TEXT NOT NULL DEFAULT '',
    weight                  REAL NOT NULL DEFAULT 0,
    height                  REAL NOT NULL DEFAULT 0,
    blood_type              TEXT NOT NULL DEFAULT '',
    address                 TEXT NOT NULL DEFAULT '',
    notes                   TEXT NOT NULL DEFAULT '',
    created_at              TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at              TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS patient_allergies (
    id         TEXT PRIMARY KEY,
    patient_id TEXT NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    allergy_id TEXT NOT NULL REFERENCES allergies(id) ON DELETE CASCADE,
    notes      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(patient_id, allergy_id)
);

CREATE TABLE IF NOT EXISTS patient_medicines (
    id          TEXT PRIMARY KEY,
    patient_id  TEXT NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    medicine_id TEXT NOT NULL REFERENCES medicines(id) ON DELETE CASCADE,
    is_active   INTEGER NOT NULL DEFAULT 1,
    notes       TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(patient_id, medicine_id)
);

-- ============================================================
-- PATIENT PROCEDURES
-- ============================================================

CREATE TABLE IF NOT EXISTS patient_procedures (
    id                      TEXT PRIMARY KEY,
    patient_id              TEXT NOT NULL REFERENCES patients(id),
    procedure_id            TEXT NOT NULL REFERENCES procedures(id),
    appointment_id          TEXT DEFAULT NULL,
    status                  TEXT NOT NULL DEFAULT 'planned' CHECK(status IN ('planned','in-progress','completed')),
    notes                   TEXT NOT NULL DEFAULT '',
    created_at              TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at              TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS patient_procedure_sessions (
    id                    TEXT PRIMARY KEY,
    patient_procedure_id  TEXT NOT NULL REFERENCES patient_procedures(id) ON DELETE CASCADE,
    procedure_session_id  TEXT NOT NULL DEFAULT '' REFERENCES procedure_sessions(id),
    appointment_id        TEXT DEFAULT NULL,
    status                TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','scheduled','completed','skipped','cancelled')),
    notes                 TEXT NOT NULL DEFAULT '',
    created_at            TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at            TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- TEAM MEMBERS (employees linked to users)
-- ============================================================

CREATE TABLE IF NOT EXISTS employees (
    id              TEXT PRIMARY KEY,
    user_id         TEXT DEFAULT '',
    first_name      TEXT NOT NULL,
    last_name       TEXT NOT NULL,
    role            TEXT NOT NULL,
    contact         TEXT NOT NULL,
    email           TEXT NOT NULL DEFAULT '',
    date_of_birth   TEXT NOT NULL DEFAULT '',
    employment_type TEXT NOT NULL CHECK(employment_type IN ('Full-time','Part-time')),
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS employee_salaries (
    id            TEXT PRIMARY KEY,
    employee_id   TEXT NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    amount        REAL NOT NULL DEFAULT 0,
    currency_id   TEXT NOT NULL DEFAULT '',
    is_active     INTEGER NOT NULL DEFAULT 1,
    effective_date TEXT NOT NULL DEFAULT '',
    notes         TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- APPOINTMENTS
-- ============================================================

CREATE TABLE IF NOT EXISTS appointments (
    id                           TEXT PRIMARY KEY,
    patient_id                   TEXT NOT NULL REFERENCES patients(id),
    room_id                      TEXT NOT NULL REFERENCES rooms(id),
    start_time                   TEXT NOT NULL,
    end_time                     TEXT NOT NULL,
    status                       TEXT NOT NULL CHECK(status IN ('Scheduled','In-Progress','Completed','Cancelled')),
    notes                        TEXT NOT NULL DEFAULT '',
    created_at                   TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at                   TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- TEMP APPOINTMENTS (external reservations, verified by staff)
-- ============================================================

CREATE TABLE IF NOT EXISTS bookings (
    id               TEXT PRIMARY KEY,
    client_name      TEXT NOT NULL,
    client_phone     TEXT NOT NULL,
    client_email     TEXT NOT NULL DEFAULT '',
    is_new_client    INTEGER NOT NULL DEFAULT 1,
    referral_source  TEXT NOT NULL DEFAULT '',
    service_category TEXT NOT NULL,
    service_name     TEXT NOT NULL,
    preferred_date   TEXT NOT NULL,
    preferred_time   TEXT NOT NULL,
    duration_minutes INTEGER NOT NULL DEFAULT 60,
    status           TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','confirmed','cancelled')),
    appointment_id   TEXT NOT NULL DEFAULT '',
    patient_id       TEXT NOT NULL DEFAULT '',
    room_id          TEXT NOT NULL DEFAULT '',
    notes            TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at       TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- SCHEDULE AVAILABILITY
-- ============================================================

CREATE TABLE IF NOT EXISTS schedule_availability (
    id             TEXT PRIMARY KEY,
    employee_id    TEXT NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    day_of_week    INTEGER NOT NULL CHECK(day_of_week BETWEEN 0 AND 6),
    start_time     TEXT NOT NULL,
    end_time       TEXT NOT NULL,
    effective_date TEXT NOT NULL DEFAULT '',
    created_at     TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- PRESCRIPTIONS
-- ============================================================

CREATE TABLE IF NOT EXISTS prescriptions (
    id                    TEXT PRIMARY KEY,
    patient_id            TEXT NOT NULL REFERENCES patients(id),
    patient_procedure_id  TEXT NOT NULL DEFAULT '',
    prescribed_by         TEXT NOT NULL DEFAULT '' REFERENCES employees(id),
    prescription_date     TEXT NOT NULL,
    created_at            TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at            TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS prescription_medicines (
    id              TEXT PRIMARY KEY,
    medicine_id     TEXT NOT NULL REFERENCES medicines(id),
    prescription_id TEXT NOT NULL REFERENCES prescriptions(id) ON DELETE CASCADE,
    instructions    TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','completed','cancelled')),
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- STOCK MANAGEMENT
-- ============================================================

CREATE TABLE IF NOT EXISTS product_categories (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    parent_id   TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS products (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    category_id   TEXT NOT NULL DEFAULT '',
    quantity      INTEGER NOT NULL DEFAULT 0,
    min_threshold INTEGER NOT NULL DEFAULT 0,
    unit_price    REAL NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS product_allergy_conflicts (
    id         TEXT PRIMARY KEY,
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    allergy_id TEXT NOT NULL REFERENCES allergies(id) ON DELETE CASCADE,
    notes      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(product_id, allergy_id)
);

-- ============================================================
-- SUPPLIERS
-- ============================================================

CREATE TABLE IF NOT EXISTS suppliers (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    contact    TEXT NOT NULL DEFAULT '',
    email      TEXT NOT NULL DEFAULT '',
    address    TEXT NOT NULL DEFAULT '',
    notes      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- FINANCIAL: BALANCES & DOUBLE-ENTRY TRANSACTIONS
-- ============================================================

CREATE TABLE IF NOT EXISTS balances (
    id          TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL CHECK(entity_type IN ('patient','employee','self','supplier')),
    entity_id   TEXT NOT NULL,
    entity_name TEXT NOT NULL DEFAULT '',
    currency_id TEXT NOT NULL DEFAULT '',
    amount      REAL NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(entity_type, entity_id, currency_id)
);

CREATE TABLE IF NOT EXISTS balance_transactions (
    id                TEXT PRIMARY KEY,
    from_balance_id   TEXT NOT NULL REFERENCES balances(id),
    to_balance_id     TEXT NOT NULL REFERENCES balances(id),
    amount            REAL NOT NULL,
    currency_id       TEXT NOT NULL DEFAULT '',
    transaction_type  TEXT NOT NULL DEFAULT 'cash' CHECK(transaction_type IN ('cash','card','transfer','other')),
    description       TEXT NOT NULL DEFAULT '',
    created_by        TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS invoices (
    id              TEXT PRIMARY KEY,
    invoice_number  INTEGER NOT NULL,
    from_balance_id TEXT NOT NULL DEFAULT '' REFERENCES balances(id),
    to_balance_id   TEXT NOT NULL DEFAULT '' REFERENCES balances(id),
    amount          REAL NOT NULL DEFAULT 0,
    currency_id     TEXT NOT NULL DEFAULT '',
    notes           TEXT NOT NULL DEFAULT '',
    created_by      TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS invoice_items (
    id         TEXT PRIMARY KEY,
    invoice_id TEXT NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    item_type  TEXT NOT NULL DEFAULT 'other' CHECK(item_type IN ('product', 'procedure', 'other')),
    item_id    TEXT NOT NULL DEFAULT '',
    quantity   INTEGER NOT NULL DEFAULT 1,
    amount     REAL NOT NULL DEFAULT 0,
    notes      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- CURRENCIES
-- ============================================================

CREATE TABLE IF NOT EXISTS currencies (
    id            TEXT PRIMARY KEY,
    code          TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    symbol        TEXT NOT NULL DEFAULT '',
    exchange_rate REAL NOT NULL DEFAULT 1.0,
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Enable foreign keys
PRAGMA foreign_keys = ON;
`

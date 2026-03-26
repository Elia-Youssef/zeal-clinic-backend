package database

const schema = `
-- ============================================================
-- SYSTEM TABLES
-- ============================================================

CREATE TABLE IF NOT EXISTS counters (
    name    TEXT PRIMARY KEY,
    value   INTEGER NOT NULL DEFAULT 0
);

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
    scopes      TEXT NOT NULL DEFAULT '[]'
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

CREATE TABLE IF NOT EXISTS notifications (
    id         TEXT PRIMARY KEY,
    type       TEXT NOT NULL CHECK(type IN ('new_booking','booking_cancelled','appointment_approved','general')),
    title      TEXT NOT NULL,
    message    TEXT NOT NULL,
    booking_id TEXT NOT NULL DEFAULT '',
    is_read    INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
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
-- PROCEDURES
-- ============================================================

CREATE TABLE IF NOT EXISTS procedures (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    procedure_type  TEXT NOT NULL DEFAULT 'Clinic Procedure' CHECK(procedure_type IN ('Clinic Procedure','Hospital Surgery','Minor Surgery')),
    category        TEXT NOT NULL DEFAULT '',
    subcategory     TEXT NOT NULL DEFAULT 'General',
    duration_minutes INTEGER NOT NULL DEFAULT 60,
    commission_rate REAL NOT NULL DEFAULT 0,
    commission_type TEXT NOT NULL DEFAULT 'percentage' CHECK(commission_type IN ('percentage','fixed')),
    is_active       INTEGER NOT NULL DEFAULT 1,
    remarks         TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS procedure_sessions (
    id              TEXT PRIMARY KEY,
    procedure_id    TEXT NOT NULL REFERENCES procedures(id) ON DELETE CASCADE,
    session_number  INTEGER NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    duration_minutes INTEGER NOT NULL DEFAULT 0,
    price           REAL NOT NULL DEFAULT 0,
    currency        TEXT NOT NULL DEFAULT 'USD',
    created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS procedure_allergy_conflicts (
    id           TEXT PRIMARY KEY,
    procedure_id TEXT NOT NULL REFERENCES procedures(id) ON DELETE CASCADE,
    allergy_id   TEXT NOT NULL REFERENCES allergies(id) ON DELETE CASCADE,
    severity     TEXT NOT NULL DEFAULT 'warning' CHECK(severity IN ('warning','critical')),
    notes        TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(procedure_id, allergy_id)
);

-- ============================================================
-- PATIENTS
-- ============================================================

CREATE TABLE IF NOT EXISTS patients (
    id                      TEXT PRIMARY KEY,
    patient_number          INTEGER NOT NULL UNIQUE,
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
    bp                      TEXT NOT NULL DEFAULT '',
    blood_type              TEXT NOT NULL DEFAULT '',
    physical_activity       TEXT NOT NULL DEFAULT 'Moderate',
    is_smoker               INTEGER NOT NULL DEFAULT 0,
    packs_per_day           REAL NOT NULL DEFAULT 0,
    on_herbal_supplements   INTEGER NOT NULL DEFAULT 0,
    on_medication           INTEGER NOT NULL DEFAULT 0,
    medication_details      TEXT NOT NULL DEFAULT '',
    on_blood_thinners       INTEGER NOT NULL DEFAULT 0,
    on_hrt                  INTEGER NOT NULL DEFAULT 0,
    hrt_details             TEXT NOT NULL DEFAULT '',
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

-- ============================================================
-- PATIENT PROCEDURES
-- ============================================================

CREATE TABLE IF NOT EXISTS patient_procedures (
    id                      TEXT PRIMARY KEY,
    patient_id              TEXT NOT NULL REFERENCES patients(id),
    procedure_id            TEXT NOT NULL REFERENCES procedures(id),
    employee_id             TEXT NOT NULL DEFAULT '' REFERENCES team_members(id),
    status                  TEXT NOT NULL DEFAULT 'planned' CHECK(status IN ('planned','in_progress','completed','cancelled')),
    referred_by_patient_id  TEXT NOT NULL DEFAULT '',
    referred_by_external    TEXT NOT NULL DEFAULT '',
    referral_commission_rate REAL NOT NULL DEFAULT 0,
    notes                   TEXT NOT NULL DEFAULT '',
    created_at              TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at              TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS patient_procedure_sessions (
    id                    TEXT PRIMARY KEY,
    patient_procedure_id  TEXT NOT NULL REFERENCES patient_procedures(id) ON DELETE CASCADE,
    procedure_session_id  TEXT NOT NULL DEFAULT '' REFERENCES procedure_sessions(id),
    appointment_id        TEXT NOT NULL DEFAULT '',
    status                TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','scheduled','completed','skipped','cancelled')),
    session_number        INTEGER NOT NULL DEFAULT 1,
    scheduled_date        TEXT NOT NULL DEFAULT '',
    completed_date        TEXT NOT NULL DEFAULT '',
    notes                 TEXT NOT NULL DEFAULT '',
    created_at            TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at            TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- TEAM MEMBERS (employees linked to users)
-- ============================================================

CREATE TABLE IF NOT EXISTS team_members (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL DEFAULT '',
    first_name      TEXT NOT NULL,
    last_name       TEXT NOT NULL,
    role            TEXT NOT NULL,
    contact         TEXT NOT NULL,
    email           TEXT NOT NULL DEFAULT '',
    date_of_birth   TEXT NOT NULL DEFAULT '',
    employment_type TEXT NOT NULL CHECK(employment_type IN ('Full-time','Part-time')),
    salary          REAL NOT NULL DEFAULT 0,
    schedule        TEXT NOT NULL DEFAULT '[]',
    off_days        TEXT NOT NULL DEFAULT '[]',
    hire_date       TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL CHECK(status IN ('Active','Inactive')),
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- APPOINTMENTS
-- ============================================================

CREATE TABLE IF NOT EXISTS appointments (
    id                          TEXT PRIMARY KEY,
    patient_id                  TEXT NOT NULL REFERENCES patients(id),
    room_id                     TEXT NOT NULL REFERENCES rooms(id),
    employee_id                 TEXT NOT NULL DEFAULT '',
    patient_procedure_session_id TEXT NOT NULL DEFAULT '',
    start_time                  TEXT NOT NULL,
    end_time                    TEXT NOT NULL,
    treatment_type              TEXT NOT NULL CHECK(treatment_type IN ('Consultation','Procedure','Follow-up')),
    status                      TEXT NOT NULL CHECK(status IN ('Scheduled','In-Progress','Completed','Cancelled')),
    approval_status             TEXT NOT NULL DEFAULT 'not_required' CHECK(approval_status IN ('not_required','pending','approved','rejected')),
    approved_by                 TEXT NOT NULL DEFAULT '',
    approved_at                 TEXT NOT NULL DEFAULT '',
    notes                       TEXT NOT NULL DEFAULT '',
    reminder_sent               INTEGER NOT NULL DEFAULT 0,
    reminder_sent_at            TEXT NOT NULL DEFAULT '',
    created_at                  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at                  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS appointment_photos (
    id             TEXT PRIMARY KEY,
    appointment_id TEXT NOT NULL REFERENCES appointments(id) ON DELETE CASCADE,
    photo_type     TEXT NOT NULL CHECK(photo_type IN ('before','after','during','other')),
    file_path      TEXT NOT NULL,
    caption        TEXT NOT NULL DEFAULT '',
    sort_order     INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT NOT NULL DEFAULT (datetime('now'))
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
    employee_id    TEXT NOT NULL REFERENCES team_members(id) ON DELETE CASCADE,
    day_of_week    INTEGER NOT NULL CHECK(day_of_week BETWEEN 0 AND 6),
    start_time     TEXT NOT NULL,
    end_time       TEXT NOT NULL,
    is_available   INTEGER NOT NULL DEFAULT 1,
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
    visit_id              TEXT NOT NULL DEFAULT '',
    prescribed_by         TEXT NOT NULL DEFAULT '',
    prescription_date     TEXT NOT NULL,
    items                 TEXT NOT NULL DEFAULT '[]',
    instructions          TEXT NOT NULL DEFAULT '',
    status                TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','completed','cancelled')),
    created_at            TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at            TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- CONSENT FORMS
-- ============================================================

CREATE TABLE IF NOT EXISTS consent_templates (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL CHECK(type IN ('general','procedure')),
    procedure   TEXT NOT NULL DEFAULT '',
    content     TEXT NOT NULL DEFAULT '',
    is_active   INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS consent_forms (
    id                   TEXT PRIMARY KEY,
    patient_id           TEXT NOT NULL REFERENCES patients(id),
    patient_procedure_id TEXT NOT NULL DEFAULT '',
    template_id          TEXT NOT NULL DEFAULT '',
    visit_id             TEXT NOT NULL DEFAULT '',
    form_type            TEXT NOT NULL CHECK(form_type IN ('general','procedure')),
    title                TEXT NOT NULL,
    content              TEXT NOT NULL DEFAULT '',
    procedure_name       TEXT NOT NULL DEFAULT '',
    signature_type       TEXT NOT NULL DEFAULT 'checkbox' CHECK(signature_type IN ('drawn','checkbox')),
    signature_data       TEXT NOT NULL DEFAULT '',
    signed_name          TEXT NOT NULL DEFAULT '',
    signed_at            TEXT NOT NULL DEFAULT '',
    status               TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','signed','revoked')),
    created_at           TEXT NOT NULL DEFAULT (datetime('now'))
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

CREATE TABLE IF NOT EXISTS inventory_items (
    sku            TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    category_id    TEXT NOT NULL DEFAULT '',
    quantity       INTEGER NOT NULL DEFAULT 0,
    min_threshold  INTEGER NOT NULL DEFAULT 0,
    unit_price     REAL NOT NULL DEFAULT 0,
    category       TEXT NOT NULL CHECK(category IN ('Consumable','Equipment','Medication')),
    last_restocked TEXT NOT NULL,
    created_at     TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS stock_adjustments (
    id              TEXT PRIMARY KEY,
    sku             TEXT NOT NULL REFERENCES inventory_items(sku),
    type            TEXT NOT NULL CHECK(type IN ('Purchase','Adjustment','Damage')),
    quantity_change INTEGER NOT NULL,
    reason          TEXT NOT NULL DEFAULT '',
    timestamp       TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS product_allergy_conflicts (
    id         TEXT PRIMARY KEY,
    sku        TEXT NOT NULL REFERENCES inventory_items(sku) ON DELETE CASCADE,
    allergy_id TEXT NOT NULL REFERENCES allergies(id) ON DELETE CASCADE,
    severity   TEXT NOT NULL DEFAULT 'warning' CHECK(severity IN ('warning','critical')),
    notes      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(sku, allergy_id)
);

-- ============================================================
-- FINANCIAL: BALANCES & DOUBLE-ENTRY TRANSACTIONS
-- ============================================================

CREATE TABLE IF NOT EXISTS balances (
    id          TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL CHECK(entity_type IN ('patient','employee','self','external')),
    entity_id   TEXT NOT NULL,
    entity_name TEXT NOT NULL DEFAULT '',
    currency    TEXT NOT NULL DEFAULT 'USD',
    amount      REAL NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(entity_type, entity_id, currency)
);

CREATE TABLE IF NOT EXISTS balance_transactions (
    id                TEXT PRIMARY KEY,
    debit_balance_id  TEXT NOT NULL REFERENCES balances(id),
    credit_balance_id TEXT NOT NULL REFERENCES balances(id),
    amount            REAL NOT NULL,
    currency          TEXT NOT NULL DEFAULT 'USD',
    exchange_rate_id  TEXT NOT NULL DEFAULT '',
    reference_type    TEXT NOT NULL DEFAULT '' CHECK(reference_type IN ('','invoice','expense','commission','refund','adjustment','salary','referral')),
    reference_id      TEXT NOT NULL DEFAULT '',
    description       TEXT NOT NULL DEFAULT '',
    created_by        TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS invoices (
    id                          TEXT PRIMARY KEY,
    invoice_number              INTEGER NOT NULL,
    type                        TEXT NOT NULL CHECK(type IN ('invoice','receipt','expense')),
    status                      TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','paid','overdue','cancelled','refunded')),
    patient_id                  TEXT NOT NULL DEFAULT '',
    patient_procedure_session_id TEXT NOT NULL DEFAULT '',
    product_sku                 TEXT NOT NULL DEFAULT '',
    balance_transaction_id      TEXT NOT NULL DEFAULT '',
    amount                      REAL NOT NULL DEFAULT 0,
    currency                    TEXT NOT NULL DEFAULT 'USD',
    payment_method              TEXT NOT NULL DEFAULT 'cash' CHECK(payment_method IN ('cash','card','insurance','bank_transfer','installments')),
    items                       TEXT NOT NULL DEFAULT '[]',
    due_date                    TEXT NOT NULL DEFAULT '',
    paid_date                   TEXT NOT NULL DEFAULT '',
    notes                       TEXT NOT NULL DEFAULT '',
    created_by                  TEXT NOT NULL DEFAULT '',
    created_at                  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at                  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- EXCHANGE RATES
-- ============================================================

CREATE TABLE IF NOT EXISTS exchange_rates (
    id             TEXT PRIMARY KEY,
    from_currency  TEXT NOT NULL DEFAULT 'USD',
    to_currency    TEXT NOT NULL DEFAULT 'LBP',
    rate           REAL NOT NULL,
    set_by         TEXT NOT NULL DEFAULT '',
    effective_date TEXT NOT NULL,
    created_at     TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- WAITLIST
-- ============================================================

CREATE TABLE IF NOT EXISTS waitlist (
    id              TEXT PRIMARY KEY,
    patient_id      TEXT NOT NULL REFERENCES patients(id),
    procedure_type  TEXT NOT NULL DEFAULT '',
    preferred_from  TEXT NOT NULL DEFAULT '',
    preferred_to    TEXT NOT NULL DEFAULT '',
    urgency         TEXT NOT NULL DEFAULT 'normal' CHECK(urgency IN ('low','normal','high')),
    notes           TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'waiting' CHECK(status IN ('waiting','contacted','booked','declined','cancelled')),
    appointment_id  TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Enable foreign keys
PRAGMA foreign_keys = ON;
`
